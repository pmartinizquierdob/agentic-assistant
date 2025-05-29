package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"time"

	"golang.org/x/oauth2"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure" // For plaintext, indev connection
	"google.golang.org/grpc/status"

	pb "github.com/pmartinizquierdob/mcp-google-services/pb" // IMPORTANT: Replace with your actual module path if different
)

const (
	mcpServerAddress = "localhost:50051"
	tokenCacheFile   = "token.json"
)

// loadAndPrepareTokens loads OAuth tokens from token.json and prepares them for gRPC request.
func loadAndPrepareTokens() (*oauth2.Token, *pb.OAuthTokens, error) {
	b, err := ioutil.ReadFile(tokenCacheFile)
	if err != nil {
		return nil, nil, fmt.Errorf("unable to read %s: %w Please ensure the MCP server has run and authorized", tokenCacheFile, err)
	}
	var tok oauth2.Token
	err = json.Unmarshal(b, &tok)
	if err != nil {
		return nil, nil, fmt.Errorf("unable to unmarshal %s: %w", tokenCacheFile, err)
	}

	// For the gRPC request, we need a protobuf-compatible structure.
	// We'll use the Unix timestamp for expiry.
	pbTokens := &pb.OAuthTokens{
		AccessToken:  tok.AccessToken,
		RefreshToken: tok.RefreshToken,
		TokenType:    tok.TokenType,
		ExpiryUnix:   tok.Expiry.Unix(),
	}
	return &tok, pbTokens, nil
}

func main() {
	log.Println("Starting MCP Client Example...")

	// 1. Load and prepare OAuth tokens
	oauthTok, pbToks, err := loadAndPrepareTokens()
	if err != nil {
		log.Fatalf("Failed to load tokens: %v", err)
	}
	log.Printf("Successfully loaded OAuth tokens (access_token: %s..., refresh_token: %s...)", oauthTok.AccessToken[:10], oauthTok.RefreshToken[:10])

	// 2. Set up a connection to the gRPC server
	conn, err := grpc.Dial(mcpServerAddress, grpc.WithTransportCredentials(insecure.NewCredentials())) // Using insecure for local dev
	if err != nil {
		log.Fatalf("Failed to connect to MCP server: %v", err)
	}
	defer conn.Close()

	// 3. Create a CalendarService client
	calendarClient := pb.NewCalendarServiceClient(conn)

	// 4. Prepare the ListEvents request
	listReq := &pb.ListEventsRequest{
		Common: &pb.CommonRequest{
			AuthTokens: pbToks, // Pass the loaded tokens
		},
		CalendarId: "primary", // Common calendar ID for the authenticated user
		MaxResults: 5,         // Get up to 5 events
	}

	// 5. Call the ListEvents RPC
	log.Println("Calling CalendarService.ListEvents...")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	res, err := calendarClient.ListEvents(ctx, listReq)
	if err != nil {
		statusErr, ok := status.FromError(err)
		if ok {
			log.Fatalf("RPC call failed with code %s: %v - %s", statusErr.Code(), statusErr.Message(), err)
		} else {
			log.Fatalf("RPC call failed: %v", err)
		}
	}

	// 6. Process the response
	if res.Common != nil && res.Common.Status == "ERROR" {
		log.Fatalf("MCP Server returned an error: %s", res.Common.Message)
	}

	log.Println("CalendarService.ListEvents successful!")
	if len(res.Events) == 0 {
		log.Println("No events found.")
	} else {
		log.Printf("Found %d events:", len(res.Events))
		for _, event := range res.Events {
			log.Printf("  - ID: %s, Summary: %s, Start: %s", event.Id, event.Summary, event.StartTime)
		}
	}

	// --- Example: Create a new event ---
	log.Println("\nCalling CalendarService.CreateEvent...")

	// Define a future time for the event
	now := time.Now()
	startTime := now.Add(2 * time.Hour).Format(time.RFC3339)
	endTime := now.Add(3 * time.Hour).Format(time.RFC3339)

	createReq := &pb.CreateEventRequest{
		Common: &pb.CommonRequest{
			AuthTokens: pbToks, // Pass the loaded tokens
		},
		CalendarId:  "primary",
		Summary:     "Reunión de Prueba LLM",
		Description: "Evento creado por el cliente gRPC de ejemplo.",
		StartTime:   startTime,
		EndTime:     endTime,
		TimeZone:    "America/Argentina/Buenos_Aires", // Adjust to your timezone
	}

	createRes, err := calendarClient.CreateEvent(ctx, createReq)
	if err != nil {
		statusErr, ok := status.FromError(err)
		if ok {
			log.Printf("RPC call CreateEvent failed with code %s: %v - %s", statusErr.Code(), statusErr.Message(), err)
		} else {
			log.Printf("RPC call CreateEvent failed: %v", err)
		}
	} else {
		if createRes.Common != nil && createRes.Common.Status == "ERROR" {
			log.Printf("MCP Server returned an error for CreateEvent: %s", createRes.Common.Message)
		} else {
			log.Printf("CalendarService.CreateEvent successful! Created Event ID: %s, Summary: %s", createRes.CreatedEvent.Id, createRes.CreatedEvent.Summary)
			log.Printf("  Link: %s", createRes.CreatedEvent.HtmlLink)
		}
	}

	// --- Example: Send an email ---
	log.Println("\nCalling GmailService.SendEmail...")
	gmailClient := pb.NewGmailServiceClient(conn)

	sendEmailReq := &pb.SendEmailRequest{
		Common: &pb.CommonRequest{
			AuthTokens: pbToks,
		},
		To:      "pmartin.izq@gmail.com", // <<--- ¡CAMBIA ESTO A UNA DIRECCIÓN DE CORREO VÁLIDA PARA PRUEBAS!
		Subject: "Prueba de envío de correo desde LLM Agent",
		Body:    "Hola, este es un correo de prueba enviado desde tu sistema MCP. ¡Funciona!",
	}

	sendEmailRes, err := gmailClient.SendEmail(ctx, sendEmailReq)
	if err != nil {
		statusErr, ok := status.FromError(err)
		if ok {
			log.Printf("RPC call SendEmail failed with code %s: %v - %s", statusErr.Code(), statusErr.Message(), err)
		} else {
			log.Printf("RPC call SendEmail failed: %v", err)
		}
	} else {
		if sendEmailRes.Common != nil && sendEmailRes.Common.Status == "ERROR" {
			log.Printf("MCP Server returned an error for SendEmail: %s", sendEmailRes.Common.Message)
		} else {
			log.Printf("GmailService.SendEmail successful! Message ID: %s", sendEmailRes.MessageId)
		}
	}

	// --- Example: List Contacts ---
	log.Println("\nCalling ContactsService.ListConnections...")
	contactsClient := pb.NewContactsServiceClient(conn)

	listContactsReq := &pb.ListConnectionsRequest{
		Common: &pb.CommonRequest{
			AuthTokens: pbToks,
		},
		PageSize: 3,
	}

	listContactsRes, err := contactsClient.ListConnections(ctx, listContactsReq)
	if err != nil {
		statusErr, ok := status.FromError(err)
		if ok {
			log.Printf("RPC call ListConnections failed with code %s: %v - %s", statusErr.Code(), statusErr.Message(), err)
		} else {
			log.Printf("RPC call ListConnections failed: %v", err)
		}
	} else {
		if listContactsRes.Common != nil && listContactsRes.Common.Status == "ERROR" {
			log.Printf("MCP Server returned an error for ListConnections: %s", listContactsRes.Common.Message)
		} else {
			log.Printf("ContactsService.ListConnections successful! Found %d contacts:", len(listContactsRes.People))
			for _, p := range listContactsRes.People {
				log.Printf("  - DisplayName: %s, Email: %s, Phone: %s", p.DisplayName, p.Email, p.PhoneNumber)
			}
		}
	}

	// --- Example: Create a Contact ---
	log.Println("\nCalling ContactsService.CreateContact...")
	createContactReq := &pb.CreateContactRequest{
		Common: &pb.CommonRequest{
			AuthTokens: pbToks,
		},
		DisplayName: "Contacto de Prueba LLM",
		Email:       "test-llm-contact@example.com", // <<--- ¡CAMBIA ESTO O AJUSTA PARA NO CREAR DUPLICADOS!
		PhoneNumber: "+1234567890",
	}

	createContactRes, err := contactsClient.CreateContact(ctx, createContactReq)
	if err != nil {
		statusErr, ok := status.FromError(err)
		if ok {
			log.Printf("RPC call CreateContact failed with code %s: %v - %s", statusErr.Code(), statusErr.Message(), err)
		} else {
			log.Printf("RPC call CreateContact failed: %v", err)
		}
	} else {
		if createContactRes.Common != nil && createContactRes.Common.Status == "ERROR" {
			log.Printf("MCP Server returned an error for CreateContact: %s", createContactRes.Common.Message)
		} else {
			log.Printf("ContactsService.CreateContact successful! Created contact: %s (%s)", createContactRes.CreatedContact.DisplayName, createContactRes.CreatedContact.Email)
		}
	}

	log.Println("\nMCP Client Example finished.")
}
