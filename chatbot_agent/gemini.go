// chatbot_agent/gemini.go
package main

import (
	"context"
	"fmt"
	"log"
	"os"

	"github.com/google/generative-ai-go/genai"
	"google.golang.org/api/option"
)

const (
	geminiAPIKeyEnv = "GEMINI_API_KEY" // Environment variable for Gemini API Key
)

var (
	geminiClient *genai.GenerativeModel
)

// InitGemini initializes the Gemini client and defines tools.
func InitGemini(ctx context.Context) error {
	apiKey := os.Getenv(geminiAPIKeyEnv)
	if apiKey == "" {
		return fmt.Errorf("GEMINI_API_KEY environment variable not set. Please set it in .env file or system environment.")
	}

	client, err := genai.NewClient(ctx, option.WithAPIKey(apiKey))
	if err != nil {
		return fmt.Errorf("error creating Gemini client: %w", err)
	}
	// No defer client.Close() here, as this is a global client for the server.

	geminiClient = client.GenerativeModel("gemini-1.5-flash-latest") // Use the latest flash model
	geminiClient.SetTemperature(0.7)                                 // Adjust as needed

	// Define the tools (functions) that Gemini can call
	geminiClient.Tools = []*genai.Tool{
		{
			FunctionDeclarations: []*genai.FunctionDeclaration{
				{
					Name:        "list_calendar_events",
					Description: "List events from the user's Google Calendar.",
					Parameters: &genai.Schema{
						Type: genai.TypeObject,
						Properties: map[string]*genai.Schema{
							"calendar_id": {
								Type:        genai.TypeString,
								Description: "The ID of the calendar to list events from (e.g., 'primary').",
							},
							"max_results": {
								Type:        genai.TypeInteger,
								Description: "Maximum number of events to return.",
							},
						},
						Required: []string{"calendar_id", "max_results"},
					},
				},
				{
					Name:        "create_calendar_event",
					Description: "Create a new event in the user's Google Calendar.",
					Parameters: &genai.Schema{
						Type: genai.TypeObject,
						Properties: map[string]*genai.Schema{
							"calendar_id": {
								Type:        genai.TypeString,
								Description: "The ID of the calendar to create the event in (e.g., 'primary').",
							},
							"summary": {
								Type:        genai.TypeString,
								Description: "Summary or title of the event.",
							},
							"description": {
								Type:        genai.TypeString,
								Description: "Description of the event.",
							},
							"start_time": {
								Type:        genai.TypeString,
								Description: "Start time of the event in RFC3339 format (e.g., '2025-05-22T15:00:00Z').",
							},
							"end_time": {
								Type:        genai.TypeString,
								Description: "End time of the event in RFC3339 format (e.g., '2025-05-22T16:00:00Z').",
							},
							"time_zone": {
								Type:        genai.TypeString,
								Description: "Time zone of the event (e.g., 'America/Argentina/Buenos_Aires').",
							},
						},
						Required: []string{"calendar_id", "summary", "start_time", "end_time", "time_zone"},
					},
				},
				{
					Name:        "send_email",
					Description: "Send an email on behalf of the user.",
					Parameters: &genai.Schema{
						Type: genai.TypeObject,
						Properties: map[string]*genai.Schema{
							"to": {
								Type:        genai.TypeString,
								Description: "Recipient's email address.",
							},
							"subject": {
								Type:        genai.TypeString,
								Description: "Subject of the email.",
							},
							"body": {
								Type:        genai.TypeString,
								Description: "Body content of the email.",
							},
						},
						Required: []string{"to", "subject", "body"},
					},
				},
				{
					Name:        "list_contacts",
					Description: "List connections (contacts) from the user's Google Contacts.",
					Parameters: &genai.Schema{
						Type: genai.TypeObject,
						Properties: map[string]*genai.Schema{
							"page_size": {
								Type:        genai.TypeInteger,
								Description: "Maximum number of contacts to return per page.",
							},
						},
						Required: []string{"page_size"},
					},
				},
				{
					Name:        "create_contact",
					Description: "Create a new contact in the user's Google Contacts.",
					Parameters: &genai.Schema{
						Type: genai.TypeObject,
						Properties: map[string]*genai.Schema{
							"display_name": {
								Type:        genai.TypeString,
								Description: "Display name of the new contact.",
							},
							"email": {
								Type:        genai.TypeString,
								Description: "Email address of the new contact.",
							},
							"phone_number": {
								Type:        genai.TypeString,
								Description: "Phone number of the new contact.",
							},
						},
						Required: []string{"display_name"}, // Email or phone can be optional
					},
				},
			},
		},
	}
	log.Println("Gemini client initialized with tools.")
	return nil
}

// GetGeminiClient returns the initialized Gemini client.
func GetGeminiClient() *genai.GenerativeModel {
	return geminiClient
}
