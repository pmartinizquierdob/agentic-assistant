// main.go
package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net"
	"net/http"
	"time"

	// Google API clients
	"google.golang.org/api/calendar/v3"
	"google.golang.org/api/gmail/v1"
	"google.golang.org/api/option"
	"google.golang.org/api/people/v1"

	// OAuth2
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"

	// gRPC
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	// Generated protobuf files
	pb "github.com/pmartinizquierdob/mcp-google-services/pb" // IMPORTANT: Replace with your actual module path if different
)

const (
	// gRPC server port
	grpcPort = ":50051"
	// OAuth2 redirect URL for the local server
	oauthRedirectURL = "http://localhost:8080/oauth2callback"
	// Token cache file for simplicity (NOT for production)
	tokenCacheFile = "token.json"
	// Credential file name
	credentialsFile = "credentials.json"
)

// Config represents the client_secrets.json structure
type Config struct {
	Web struct {
		ClientID                string   `json:"client_id"`
		ProjectID               string   `json:"project_id"`
		AuthURI                 string   `json:"auth_uri"`
		TokenURI                string   `json:"token_uri"`
		AuthProviderX509CertURL string   `json:"auth_provider_x509_cert_url"`
		ClientSecret            string   `json:"client_secret"`
		RedirectURIs            []string `json:"redirect_uris"`
	} `json:"web"`
}

// global variables for simplicity in this single file example
var (
	googleOAuthConfig *oauth2.Config
	// tokenStore stores tokens per user or session in a real app.
	// For this example, we'll manage a single token in the context of gRPC calls.
	// In a real app, you'd load/save this from a database based on a user ID.
)

// Helper to get an OAuth2 token from the request, or initiate a new flow
func getTokenFromRequest(ctx context.Context, commonReq *pb.CommonRequest) (*oauth2.Token, error) {
	if commonReq == nil || commonReq.AuthTokens == nil {
		return nil, status.Errorf(codes.Unauthenticated, "No OAuth tokens provided in request.")
	}

	authTokens := commonReq.AuthTokens
	tok := &oauth2.Token{
		AccessToken:  authTokens.AccessToken,
		RefreshToken: authTokens.RefreshToken,
		TokenType:    authTokens.TokenType,
		Expiry:       time.Unix(authTokens.ExpiryUnix, 0),
	}

	// Create a token source with the provided token.
	// This token source will handle refreshing the token if it's expired
	// and a refresh token is available.
	tokenSource := googleOAuthConfig.TokenSource(ctx, tok)

	// Attempt to get a fresh token. If the token is expired and a refresh token
	// is available, it will refresh. If not, it will return an error.
	freshTok, err := tokenSource.Token()
	if err != nil {
		log.Printf("Error getting fresh token: %v", err)
		return nil, status.Errorf(codes.Unauthenticated, "Failed to get fresh token: %v. Please re-authenticate.", err)
	}

	// If the token was refreshed, update the client with the new token details
	if freshTok.AccessToken != tok.AccessToken || freshTok.Expiry.Unix() != tok.Expiry.Unix() {
		log.Println("Token was refreshed.")
		// In a real application, you would persist freshTok.RefreshToken and other details
		// associated with the user who made the original request.
		// For this example, we'll just return the fresh token and assume the client
		// (e.g., the Multiple MCP Client) will handle persisting it if needed.
	}

	return freshTok, nil
}

// Function to handle the OAuth2 callback (for initial token acquisition)
func handleOAuth2Callback(w http.ResponseWriter, r *http.Request) {
	code := r.URL.Query().Get("code")
	if code == "" {
		log.Printf("No 'code' parameter in OAuth2 callback: %v", r.URL.Query())
		http.Error(w, "Authorization code not found.", http.StatusBadRequest)
		return
	}

	tok, err := googleOAuthConfig.Exchange(r.Context(), code)
	if err != nil {
		log.Printf("Unable to retrieve token from web: %v", err)
		http.Error(w, fmt.Sprintf("Unable to retrieve token from web: %v", err), http.StatusInternalServerError)
		return
	}

	// For simplicity, save token to a file. In a real app, this would be persisted securely.
	b, err := json.MarshalIndent(tok, "", "  ")
	if err != nil {
		log.Printf("Unable to marshal token: %v", err)
		http.Error(w, "Internal server error.", http.StatusInternalServerError)
		return
	}
	if err := ioutil.WriteFile(tokenCacheFile, b, 0600); err != nil {
		log.Printf("Unable to cache OAuth token: %v", err)
		http.Error(w, "Internal server error.", http.StatusInternalServerError)
		return
	}

	fmt.Fprintf(w, "Authentication successful! Your tokens have been saved to %s. You can now make gRPC calls.", tokenCacheFile)
	log.Println("OAuth token saved to token.json")
}

// ====================================================================
// Calendar Service Implementation
// ====================================================================
type calendarServer struct {
	pb.UnimplementedCalendarServiceServer
}

func (s *calendarServer) ListEvents(ctx context.Context, req *pb.ListEventsRequest) (*pb.ListEventsResponse, error) {
	tok, err := getTokenFromRequest(ctx, req.Common)
	if err != nil {
		return nil, err
	}

	client := googleOAuthConfig.Client(ctx, tok)
	srv, err := calendar.NewService(ctx, option.WithHTTPClient(client))
	if err != nil {
		return nil, status.Errorf(codes.Internal, "Unable to retrieve Calendar client: %v", err)
	}

	t := time.Now().Add(-24 * time.Hour).Format(time.RFC3339) // Events from yesterday
	events, err := srv.Events.List(req.CalendarId).ShowDeleted(false).SingleEvents(true).TimeMin(t).MaxResults(int64(req.MaxResults)).OrderBy("startTime").Do()
	if err != nil {
		return nil, status.Errorf(codes.Internal, "Unable to retrieve calendar events: %v", err)
	}

	var pbEvents []*pb.Event
	for _, item := range events.Items {
		start := ""
		if item.Start.DateTime != "" {
			start = item.Start.DateTime
		} else {
			start = item.Start.Date
		}
		end := ""
		if item.End.DateTime != "" {
			end = item.End.DateTime
		} else {
			end = item.End.Date
		}

		pbEvents = append(pbEvents, &pb.Event{
			Id:          item.Id,
			Summary:     item.Summary,
			Description: item.Description,
			StartTime:   start,
			EndTime:     end,
			HtmlLink:    item.HtmlLink,
		})
	}

	return &pb.ListEventsResponse{
		Common: &pb.CommonResponse{Status: "OK", Message: "Events listed successfully."},
		Events: pbEvents,
	}, nil
}

func (s *calendarServer) CreateEvent(ctx context.Context, req *pb.CreateEventRequest) (*pb.CreateEventResponse, error) {
	tok, err := getTokenFromRequest(ctx, req.Common)
	if err != nil {
		return nil, err
	}

	client := googleOAuthConfig.Client(ctx, tok)
	srv, err := calendar.NewService(ctx, option.WithHTTPClient(client))
	if err != nil {
		return nil, status.Errorf(codes.Internal, "Unable to retrieve Calendar client: %v", err)
	}

	event := &calendar.Event{
		Summary:     req.Summary,
		Description: req.Description,
		Start: &calendar.EventDateTime{
			DateTime: req.StartTime,
			TimeZone: req.TimeZone,
		},
		End: &calendar.EventDateTime{
			DateTime: req.EndTime,
			TimeZone: req.TimeZone,
		},
	}

	newEvent, err := srv.Events.Insert(req.CalendarId, event).Do()
	if err != nil {
		return nil, status.Errorf(codes.Internal, "Unable to create calendar event: %v", err)
	}

	pbEvent := &pb.Event{
		Id:          newEvent.Id,
		Summary:     newEvent.Summary,
		Description: newEvent.Description,
		StartTime:   newEvent.Start.DateTime,
		EndTime:     newEvent.End.DateTime,
		HtmlLink:    newEvent.HtmlLink,
	}

	return &pb.CreateEventResponse{
		Common:       &pb.CommonResponse{Status: "OK", Message: "Event created successfully."},
		CreatedEvent: pbEvent,
	}, nil
}

// ====================================================================
// Gmail Service Implementation
// ====================================================================
type gmailServer struct {
	pb.UnimplementedGmailServiceServer
}

func (s *gmailServer) SendEmail(ctx context.Context, req *pb.SendEmailRequest) (*pb.SendEmailResponse, error) {
	tok, err := getTokenFromRequest(ctx, req.Common)
	if err != nil {
		return nil, err
	}

	client := googleOAuthConfig.Client(ctx, tok)
	srv, err := gmail.NewService(ctx, option.WithHTTPClient(client))
	if err != nil {
		return nil, status.Errorf(codes.Internal, "Unable to retrieve Gmail client: %v", err)
	}

	var message gmail.Message
	mimeMessage := []byte(fmt.Sprintf("To: %s\r\nSubject: %s\r\n\r\n%s", req.To, req.Subject, req.Body))
	message.Raw = base64.URLEncoding.EncodeToString(mimeMessage)

	_, err = srv.Users.Messages.Send("me", &message).Do()
	if err != nil {
		return nil, status.Errorf(codes.Internal, "Unable to send email: %v", err)
	}

	return &pb.SendEmailResponse{
		Common:    &pb.CommonResponse{Status: "OK", Message: "Email sent successfully."},
		MessageId: message.Id, // Gmail API populates message.Id after sending
	}, nil
}

func (s *gmailServer) ListMessages(ctx context.Context, req *pb.ListMessagesRequest) (*pb.ListMessagesResponse, error) {
	tok, err := getTokenFromRequest(ctx, req.Common)
	if err != nil {
		return nil, err
	}

	client := googleOAuthConfig.Client(ctx, tok)
	srv, err := gmail.NewService(ctx, option.WithHTTPClient(client))
	if err != nil {
		return nil, status.Errorf(codes.Internal, "Unable to retrieve Gmail client: %v", err)
	}

	call := srv.Users.Messages.List("me")
	if req.MaxResults > 0 {
		call.MaxResults(int64(req.MaxResults))
	}
	if req.Query != "" {
		call.Q(req.Query)
	}

	msgs, err := call.Do()
	if err != nil {
		return nil, status.Errorf(codes.Internal, "Unable to list messages: %v", err)
	}

	var pbMessages []*pb.Message
	for _, msg := range msgs.Messages {
		pbMessages = append(pbMessages, &pb.Message{
			Id:       msg.Id,
			Snippet:  msg.Snippet,
			LabelIds: msg.LabelIds,
		})
	}

	return &pb.ListMessagesResponse{
		Common:   &pb.CommonResponse{Status: "OK", Message: "Messages listed successfully."},
		Messages: pbMessages,
	}, nil
}

func (s *gmailServer) GetMessage(ctx context.Context, req *pb.GetMessageRequest) (*pb.GetMessageResponse, error) {
	tok, err := getTokenFromRequest(ctx, req.Common)
	if err != nil {
		return nil, err
	}

	client := googleOAuthConfig.Client(ctx, tok)
	srv, err := gmail.NewService(ctx, option.WithHTTPClient(client))
	if err != nil {
		return nil, status.Errorf(codes.Internal, "Unable to retrieve Gmail client: %v", err)
	}

	msg, err := srv.Users.Messages.Get("me", req.MessageId).Format("full").Do()
	if err != nil {
		return nil, status.Errorf(codes.Internal, "Unable to get message: %v", err)
	}

	// Extract headers
	var subject, from, to, date string
	for _, h := range msg.Payload.Headers {
		switch h.Name {
		case "Subject":
			subject = h.Value
		case "From":
			from = h.Value
		case "To":
			to = h.Value
		case "Date":
			date = h.Value
		}
	}

	// Extract body
	var body string
	if msg.Payload.Parts != nil {
		for _, part := range msg.Payload.Parts {
			if part.MimeType == "text/plain" || part.MimeType == "text/html" {
				decodedData, _ := base64.URLEncoding.DecodeString(part.Body.Data)
				body = string(decodedData)
				break
			}
		}
	} else if msg.Payload.Body != nil && msg.Payload.Body.Data != "" {
		decodedData, _ := base64.URLEncoding.DecodeString(msg.Payload.Body.Data)
		body = string(decodedData)
	}

	return &pb.GetMessageResponse{
		Common:    &pb.CommonResponse{Status: "OK", Message: "Message retrieved successfully."},
		MessageId: msg.Id,
		Subject:   subject,
		From:      from,
		To:        to,
		Date:      date,
		Body:      body,
	}, nil
}

// ====================================================================
// Contacts Service Implementation
// ====================================================================
type contactsServer struct {
	pb.UnimplementedContactsServiceServer
}

func (s *contactsServer) ListConnections(ctx context.Context, req *pb.ListConnectionsRequest) (*pb.ListConnectionsResponse, error) {
	tok, err := getTokenFromRequest(ctx, req.Common)
	if err != nil {
		return nil, err
	}

	client := googleOAuthConfig.Client(ctx, tok)
	srv, err := people.NewService(ctx, option.WithHTTPClient(client))
	if err != nil {
		return nil, status.Errorf(codes.Internal, "Unable to retrieve People client: %v", err)
	}

	call := srv.People.Connections.List("people/me").
		PersonFields("names,emailAddresses,phoneNumbers").
		PageSize(int64(req.PageSize))

	connections, err := call.Do()
	if err != nil {
		return nil, status.Errorf(codes.Internal, "Unable to list connections: %v", err)
	}

	var pbPeople []*pb.Person
	for _, person := range connections.Connections {
		var name, email, phone string
		if len(person.Names) > 0 {
			name = person.Names[0].DisplayName
		}
		if len(person.EmailAddresses) > 0 {
			email = person.EmailAddresses[0].Value
		}
		if len(person.PhoneNumbers) > 0 {
			phone = person.PhoneNumbers[0].Value
		}

		pbPeople = append(pbPeople, &pb.Person{
			ResourceName: person.ResourceName,
			DisplayName:  name,
			Email:        email,
			PhoneNumber:  phone,
		})
	}

	return &pb.ListConnectionsResponse{
		Common: &pb.CommonResponse{Status: "OK", Message: "Connections listed successfully."},
		People: pbPeople,
	}, nil
}

func (s *contactsServer) CreateContact(ctx context.Context, req *pb.CreateContactRequest) (*pb.CreateContactResponse, error) {
	tok, err := getTokenFromRequest(ctx, req.Common)
	if err != nil {
		return nil, err
	}

	client := googleOAuthConfig.Client(ctx, tok)
	srv, err := people.NewService(ctx, option.WithHTTPClient(client))
	if err != nil {
		return nil, status.Errorf(codes.Internal, "Unable to retrieve People client: %v", err)
	}

	contact := &people.Person{
		Names: []*people.Name{
			{
				DisplayName: req.DisplayName,
			},
		},
	}
	if req.Email != "" {
		contact.EmailAddresses = []*people.EmailAddress{
			{
				Value: req.Email,
			},
		}
	}
	if req.PhoneNumber != "" {
		contact.PhoneNumbers = []*people.PhoneNumber{
			{
				Value: req.PhoneNumber,
			},
		}
	}

	createdPerson, err := srv.People.CreateContact(contact).Do()
	if err != nil {
		return nil, status.Errorf(codes.Internal, "Unable to create contact: %v", err)
	}

	// Ejemplo de cómo manejar el acceso seguro:
	var email string
	if createdPerson.EmailAddresses != nil && len(createdPerson.EmailAddresses) > 0 {
		email = createdPerson.EmailAddresses[0].Value
	} else {
		email = "" // O maneja el caso de que no haya email
	}

	var phoneNumber string
	if createdPerson.PhoneNumbers != nil && len(createdPerson.PhoneNumbers) > 0 {
		phoneNumber = createdPerson.PhoneNumbers[0].Value
	} else {
		phoneNumber = "" // O maneja el caso de que no haya número de teléfono
	}

	// Para DisplayName, el Names[0].DisplayName debería estar presente si se envió en la creación
	// pero siempre es buena práctica verificar:
	var displayName string
	if createdPerson.Names != nil && len(createdPerson.Names) > 0 {
		displayName = createdPerson.Names[0].DisplayName
	} else {
		displayName = req.DisplayName // Usa el que se envió en la solicitud si no hay DisplayName de vuelta
	}

	pbPerson := &pb.Person{
		DisplayName:  displayName,
		Email:        email,       // Usa la variable segura
		PhoneNumber:  phoneNumber, // Usa la variable segura
		ResourceName: createdPerson.ResourceName,
	}

	return &pb.CreateContactResponse{
		Common:         &pb.CommonResponse{Status: "OK", Message: "Contact created successfully."},
		CreatedContact: pbPerson,
	}, nil
}

// main function to set up and run the gRPC server and OAuth2 callback handler
func main() {
	log.Println("Starting MCP Services Server...")

	// Load Google API client credentials
	b, err := ioutil.ReadFile(credentialsFile)
	if err != nil {
		log.Fatalf("Unable to read client secret file (%s): %v. Please download your 'client_secret.json' from Google Cloud Console and rename it to 'credentials.json'.", credentialsFile, err)
	}

	var cfg Config
	err = json.Unmarshal(b, &cfg)
	if err != nil {
		log.Fatalf("Unable to parse client secret file to config: %v", err)
	}

	// Configure OAuth2
	googleOAuthConfig = &oauth2.Config{
		ClientID:     cfg.Web.ClientID,
		ClientSecret: cfg.Web.ClientSecret,
		RedirectURL:  oauthRedirectURL,
		Scopes: []string{
			calendar.CalendarEventsScope, // Full access to Calendar events
			gmail.GmailModifyScope,       // Full access to Gmail messages, including sending
			people.ContactsScope,         // Full access to Contacts
		},
		Endpoint: google.Endpoint,
	}

	// Start a simple HTTP server for OAuth2 callback
	go func() {
		http.HandleFunc("/oauth2callback", handleOAuth2Callback)
		log.Printf("Starting OAuth2 callback handler on %s...", oauthRedirectURL)
		log.Fatal(http.ListenAndServe(":8080", nil)) // Listen on port 8080 for OAuth callback
	}()

	// Print the URL to authorize
	authURL := googleOAuthConfig.AuthCodeURL("state-token", oauth2.AccessTypeOffline, oauth2.ApprovalForce)
	log.Printf("Go to the following link in your browser to authorize your Google account:\n%s", authURL)
	log.Println("After authorization, the tokens will be saved to token.json in the current directory.")

	// Set up gRPC server
	lis, err := net.Listen("tcp", grpcPort)
	if err != nil {
		log.Fatalf("Failed to listen: %v", err)
	}

	s := grpc.NewServer()
	pb.RegisterCalendarServiceServer(s, &calendarServer{})
	pb.RegisterGmailServiceServer(s, &gmailServer{})
	pb.RegisterContactsServiceServer(s, &contactsServer{})

	log.Printf("gRPC server listening at %v", lis.Addr())
	if err := s.Serve(lis); err != nil {
		log.Fatalf("Failed to serve: %v", err)
	}
}
