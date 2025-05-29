// chatbot_agent/types.go
package main

import (
	"github.com/google/generative-ai-go/genai"
	pb "github.com/pmartinizquierdob/mcp-google-services/pb" // Ensure this path is correct
)

// UserSession stores chat session and OAuth tokens for a user.
type UserSession struct {
	ChatSession *genai.ChatSession
	OAuthTokens *pb.OAuthTokens // Stores the last known valid tokens for the user
	// Add other session data as needed
}

// WhatsAppWebhookPayload simulates the incoming WhatsApp message structure
type WhatsAppWebhookPayload struct {
	Object string `json:"object"`
	Entry  []struct {
		ID      string `json:"id"`
		Changes []struct {
			Value struct {
				MessagingProduct string `json:"messaging_product"`
				Metadata         struct {
					DisplayPhoneNumberID string `json:"display_phone_number_id"`
					PhoneNumberID        string `json:"phone_number_id"`
				} `json:"metadata"`
				Contacts []struct {
					Profile struct {
						Name string `json:"name"`
					} `json:"profile"`
					WaID string `json:"wa_id"` // User's WhatsApp ID
				} `json:"contacts"`
				Messages []struct {
					From      string `json:"from"` // User's WhatsApp ID
					ID        string `json:"id"`
					Timestamp string `json:"timestamp"`
					Text      struct {
						Body string `json:"body"`
					} `json:"text"`
					Type string `json:"type"`
				} `json:"messages"`
			} `json:"value"`
			Field string `json:"field"`
		} `json:"changes"`
	} `json:"entry"`
}

// OutgoingWhatsAppMessage simulates sending a message back
type OutgoingWhatsAppMessage struct {
	MessagingProduct string `json:"messaging_product"`
	To               string `json:"to"`
	Type             string `json:"type"`
	Text             struct {
		Body string `json:"body"`
	} `json:"text"`
}
