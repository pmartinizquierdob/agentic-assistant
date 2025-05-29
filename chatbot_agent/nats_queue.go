// chatbot_agent/nats_queue.go
package main

import (
	"encoding/json"
	"fmt"
	"log"
	"time"

	"github.com/nats-io/nats.go"
)

const (
	natsURL                   = nats.DefaultURL
	natsSubject               = "incoming.messages"
	natsResponseSubjectPrefix = "response.messages." // response.messages.<user_id>
)

// PublishIncomingMessage publishes an incoming WhatsApp payload to NATS.
func PublishIncomingMessage(nc *nats.Conn, payload WhatsAppWebhookPayload) error {
	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("error marshalling WhatsApp payload: %w", err)
	}
	if err := nc.Publish(natsSubject, payloadBytes); err != nil {
		return fmt.Errorf("error publishing incoming webhook to NATS: %w", err)
	}
	log.Printf("Published incoming webhook to NATS.")
	return nil
}

// SubscribeToIncomingMessages sets up a NATS subscriber for incoming messages.
func SubscribeToIncomingMessages(nc *nats.Conn, handler func(msg *nats.Msg)) (*nats.Subscription, error) {
	sub, err := nc.Subscribe(natsSubject, handler)
	if err != nil {
		return nil, fmt.Errorf("failed to subscribe to NATS subject '%s': %w", natsSubject, err)
	}
	log.Printf("Subscribed to NATS subject '%s' for incoming messages.", natsSubject)
	return sub, nil
}

// SendResponse publishes the chatbot's response to a NATS subject for the specific user.
func SendResponse(nc *nats.Conn, userID, message string) {
	respMsg := OutgoingWhatsAppMessage{
		MessagingProduct: "whatsapp",
		To:               userID,
		Type:             "text",
		Text: struct {
			Body string `json:"body"`
		}{Body: message},
	}
	respBytes, _ := json.Marshal(respMsg)
	subject := natsResponseSubjectPrefix + userID
	if err := nc.Publish(subject, respBytes); err != nil {
		log.Printf("Error publishing response to NATS subject '%s': %v", subject, err)
	} else {
		log.Printf("Published response to NATS for user %s: '%s'", userID, message)
	}
}

// GetResponseFromNATS waits for a response from NATS for a specific user ID.
func GetResponseFromNATS(nc *nats.Conn, userID string, timeout time.Duration) (string, error) {
	msgChan := make(chan string)
	subject := natsResponseSubjectPrefix + userID

	sub, err := nc.Subscribe(subject, func(msg *nats.Msg) {
		log.Printf("Received response from NATS for user %s: %s", userID, string(msg.Data))
		var outgoingMsg OutgoingWhatsAppMessage
		if jsonErr := json.Unmarshal(msg.Data, &outgoingMsg); jsonErr == nil {
			msgChan <- outgoingMsg.Text.Body
		} else {
			log.Printf("Error unmarshalling outgoing WhatsApp message: %v", jsonErr)
			msgChan <- "Error processing response."
		}
		msg.Sub.Unsubscribe() // Unsubscribe after receiving one message
	})
	if err != nil {
		return "", fmt.Errorf("failed to subscribe for response: %w", err)
	}
	defer sub.Unsubscribe() // Ensure unsubscribe if response is not received

	select {
	case responseText := <-msgChan:
		return responseText, nil
	case <-time.After(timeout):
		return "", fmt.Errorf("response timeout")
	}
}
