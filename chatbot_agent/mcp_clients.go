// chatbot_agent/mcp_clients.go
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"time"

	"golang.org/x/oauth2" // For loading token.json
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	pb "github.com/pmartinizquierdob/mcp-google-services/pb" // Ensure this path is correct
)

const (
	mcpServerAddress = "localhost:50051" // Address of your MCP gRPC server
	tokenCacheFile   = "token.json"      // Location of the token.json for the chatbot
)

var (
	mcpCalendarClient pb.CalendarServiceClient
	mcpGmailClient    pb.GmailServiceClient
	mcpContactsClient pb.ContactsServiceClient
)

// InitMCPClients initializes gRPC clients for the MCP services.
func InitMCPClients(ctx context.Context) error {
	conn, err := grpc.Dial(mcpServerAddress, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return fmt.Errorf("failed to connect to MCP server at %s: %w", mcpServerAddress, err)
	}
	// Do not defer conn.Close() here, as this connection is intended to be long-lived
	// for the duration of the chatbot server's life. Close it in main cleanup if needed.

	mcpCalendarClient = pb.NewCalendarServiceClient(conn)
	mcpGmailClient = pb.NewGmailServiceClient(conn)
	mcpContactsClient = pb.NewContactsServiceClient(conn)
	log.Println("MCP gRPC clients initialized.")
	return nil
}

// ExecuteToolCall dispatches the tool call to the appropriate MCP client.
func ExecuteToolCall(ctx context.Context, userID string, tokens *pb.OAuthTokens, toolName string, args map[string]interface{}) (interface{}, error) {
	commonReq := &pb.CommonRequest{AuthTokens: tokens}

	rpcCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()

	switch toolName {
	case "list_calendar_events":
		calendarID := "primary"
		if val, ok := args["calendar_id"].(string); ok {
			calendarID = val
		}
		maxResults := int32(10)                           // Default
		if val, ok := args["max_results"].(float64); ok { // JSON numbers are float64 in Go interface{}
			maxResults = int32(val)
		}
		req := &pb.ListEventsRequest{
			Common:     commonReq,
			CalendarId: calendarID,
			MaxResults: maxResults,
		}
		resp, err := mcpCalendarClient.ListEvents(rpcCtx, req)
		if err != nil {
			return nil, fmt.Errorf("list_calendar_events RPC failed: %w", err)
		}
		if resp.Common.Status == "ERROR" {
			return nil, fmt.Errorf("list_calendar_events MCP error: %s", resp.Common.Message)
		}
		var eventSummaries []string
		for _, event := range resp.Events {
			eventSummaries = append(eventSummaries, fmt.Sprintf("ID: %s, Summary: '%s', Start: %s", event.Id, event.Summary, event.StartTime))
		}
		return map[string]interface{}{"events": eventSummaries}, nil

	case "create_calendar_event":
		// Extract all required arguments, handle type assertions
		calendarID, _ := args["calendar_id"].(string)
		summary, _ := args["summary"].(string)
		description, _ := args["description"].(string)
		startTime, _ := args["start_time"].(string)
		endTime, _ := args["end_time"].(string)
		timeZone, _ := args["time_zone"].(string)

		req := &pb.CreateEventRequest{
			Common:      commonReq,
			CalendarId:  calendarID,
			Summary:     summary,
			Description: description,
			StartTime:   startTime,
			EndTime:     endTime,
			TimeZone:    timeZone,
		}
		resp, err := mcpCalendarClient.CreateEvent(rpcCtx, req)
		if err != nil {
			return nil, fmt.Errorf("create_calendar_event RPC failed: %w", err)
		}
		if resp.Common.Status == "ERROR" {
			return nil, fmt.Errorf("create_calendar_event MCP error: %s", resp.Common.Message)
		}
		return map[string]interface{}{"event_id": resp.CreatedEvent.Id, "summary": resp.CreatedEvent.Summary, "link": resp.CreatedEvent.HtmlLink}, nil

	case "send_email":
		to, _ := args["to"].(string)
		subject, _ := args["subject"].(string)
		body, _ := args["body"].(string)

		req := &pb.SendEmailRequest{
			Common:  commonReq,
			To:      to,
			Subject: subject,
			Body:    body,
		}
		resp, err := mcpGmailClient.SendEmail(rpcCtx, req)
		if err != nil {
			return nil, fmt.Errorf("send_email RPC failed: %w", err)
		}
		if resp.Common.Status == "ERROR" {
			return nil, fmt.Errorf("send_email MCP error: %s", resp.Common.Message)
		}
		return map[string]interface{}{"message_id": resp.MessageId}, nil

	case "list_contacts":
		pageSize := int32(10) // Default
		if val, ok := args["page_size"].(float64); ok {
			pageSize = int32(val)
		}
		req := &pb.ListConnectionsRequest{
			Common:   commonReq,
			PageSize: pageSize,
		}
		resp, err := mcpContactsClient.ListConnections(rpcCtx, req)
		if err != nil {
			return nil, fmt.Errorf("list_contacts RPC failed: %w", err)
		}
		if resp.Common.Status == "ERROR" {
			return nil, fmt.Errorf("list_contacts MCP error: %s", resp.Common.Message)
		}
		var contactSummaries []string
		for _, p := range resp.People {
			contactSummaries = append(contactSummaries, fmt.Sprintf("Name: %s, Email: %s, Phone: %s", p.DisplayName, p.Email, p.PhoneNumber))
		}
		return map[string]interface{}{"contacts": contactSummaries}, nil

	case "create_contact":
		displayName, _ := args["display_name"].(string)
		email, _ := args["email"].(string)
		phoneNumber, _ := args["phone_number"].(string)

		req := &pb.CreateContactRequest{
			Common:      commonReq,
			DisplayName: displayName,
			Email:       email,
			PhoneNumber: phoneNumber,
		}
		resp, err := mcpContactsClient.CreateContact(rpcCtx, req)
		if err != nil {
			return nil, fmt.Errorf("create_contact RPC failed: %w", err)
		}
		if resp.Common.Status == "ERROR" {
			return nil, fmt.Errorf("create_contact MCP error: %s", resp.Common.Message)
		}
		return map[string]interface{}{"contact_name": resp.CreatedContact.DisplayName, "contact_id": resp.CreatedContact.ResourceName}, nil

	default:
		return nil, fmt.Errorf("unknown tool: %s", toolName)
	}
}

// loadAndPrepareTokens loads OAuth tokens from token.json and prepares them for gRPC request.
// This function is kept here as it's specific to loading tokens for MCP client use.
func LoadAndPrepareTokens() (*oauth2.Token, *pb.OAuthTokens, error) {
	b, err := ioutil.ReadFile(tokenCacheFile)
	if err != nil {
		return nil, nil, fmt.Errorf("unable to read %s: %w. Please ensure the MCP server has run and authorized.", tokenCacheFile, err)
	}
	var tok oauth2.Token
	err = json.Unmarshal(b, &tok)
	if err != nil {
		return nil, nil, fmt.Errorf("unable to unmarshal %s: %w", tokenCacheFile, err)
	}

	pbTokens := &pb.OAuthTokens{
		AccessToken:  tok.AccessToken,
		RefreshToken: tok.RefreshToken,
		TokenType:    tok.TokenType,
		ExpiryUnix:   tok.Expiry.Unix(),
	}
	return &tok, pbTokens, nil
}
