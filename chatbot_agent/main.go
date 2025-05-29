// chatbot_agent/main.go
package main

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/generative-ai-go/genai"
	"github.com/joho/godotenv"
	"github.com/nats-io/nats.go"
	// Required for status.FromError
)

const (
	chatbotPort = ":8082" // Port for the webhook endpoint
)

func main() {
	log.Println("Starting Chatbot Server (Client Face Layer)...")

	// Load environment variables from .env file
	err := godotenv.Load()
	if err != nil {
		log.Printf("Warning: No .env file found or error loading .env: %v. Proceeding with system environment variables.", err)
	}

	// Initialize NATS connection
	nc, err := nats.Connect(natsURL)
	if err != nil {
		log.Fatalf("Failed to connect to NATS: %v", err)
	}
	defer nc.Close()
	log.Println("Connected to NATS server.")

	// Context for initializations and graceful shutdown
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Initialize Gemini AI client and tools
	if err := InitGemini(ctx); err != nil { // Use the exported InitGemini
		log.Fatalf("Failed to initialize Gemini client: %v", err)
	}

	// Initialize MCP gRPC clients
	if err := InitMCPClients(ctx); err != nil { // Use the exported InitMCPClients
		log.Fatalf("Failed to initialize MCP gRPC clients: %v", err)
	}

	// Set up NATS consumer for incoming messages
	_, err = SubscribeToIncomingMessages(nc, func(msg *nats.Msg) { // Use exported SubscribeToIncomingMessages
		log.Printf("Received message from NATS: %s", string(msg.Data))
		var whatsappPayload WhatsAppWebhookPayload
		if err := json.Unmarshal(msg.Data, &whatsappPayload); err != nil {
			log.Printf("Error unmarshalling WhatsApp payload from NATS: %v", err)
			return
		}

		if len(whatsappPayload.Entry) > 0 && len(whatsappPayload.Entry[0].Changes) > 0 &&
			len(whatsappPayload.Entry[0].Changes[0].Value.Messages) > 0 {

			message := whatsappPayload.Entry[0].Changes[0].Value.Messages[0]
			userID := message.From
			textBody := message.Text.Body

			if userID != "" && textBody != "" {
				go processMessage(ctx, userID, textBody, nc) // Process message in a goroutine
			} else {
				log.Println("Could not extract user ID or message text from WhatsApp payload.")
			}
		}
	})
	if err != nil {
		log.Fatalf("Failed to subscribe to NATS subject '%s': %v", natsSubject, err)
	}

	// Set up Gin HTTP server for incoming webhooks (simulated WhatsApp)
	router := gin.Default() // router is now properly initialized here

	router.POST("/whatsapp/webhook", func(c *gin.Context) {
		var payload WhatsAppWebhookPayload
		if err := c.ShouldBindJSON(&payload); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid payload", "details": err.Error()})
			return
		}

		if err := PublishIncomingMessage(nc, payload); err != nil { // Use exported PublishIncomingMessage
			log.Printf("Error publishing incoming webhook to NATS: %v", err)
			c.JSON(http.StatusInternalServerError, gin.H{"status": "error", "message": "Failed to queue message"})
			return
		}

		c.JSON(http.StatusOK, gin.H{"status": "ok", "message": "Message received and queued"})
	})

	router.GET("/response/:user_id", func(c *gin.Context) {
		userID := c.Param("user_id")
		responseText, err := GetResponseFromNATS(nc, userID, 15*time.Second) // Use exported GetResponseFromNATS
		if err != nil {
			c.JSON(http.StatusRequestTimeout, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"user_id": userID, "response": responseText})
	})

	log.Printf("Chatbot server listening on %s", chatbotPort)
	log.Fatal(router.Run(chatbotPort))
}

// chatbot_agent/main.go (Modificación en la función processMessage)

func processMessage(ctx context.Context, userID, text string, nc *nats.Conn) {
	log.Printf("Processing message for user %s: '%s'", userID, text)

	session, _ := GetOrCreateUserSession(userID)

	if session.ChatSession == nil {
		session.ChatSession = GetGeminiClient().StartChat()
	}

	if session.OAuthTokens == nil {
		log.Printf("Attempting to load initial OAuth tokens for user %s...", userID)
		_, loadedPBTokens, err := LoadAndPrepareTokens()
		if err != nil {
			log.Printf("Error loading initial tokens for user %s: %v. Please ensure token.json exists.", userID, err)
			SendResponse(nc, userID, "Lo siento, necesito que autorices tu cuenta de Google. Puedes hacerlo siguiendo las instrucciones del servidor MCP. (Error: "+err.Error()+")")
			return
		}
		UpdateUserSessionTokens(userID, loadedPBTokens)
		log.Printf("Successfully loaded initial OAuth tokens for user %s.", userID)
	}

	geminiChatSession := session.ChatSession
	geminiChatSession.History = append(geminiChatSession.History, &genai.Content{
		Parts: []genai.Part{genai.Text(text)},
		Role:  "user",
	})

	resp, err := geminiChatSession.SendMessage(ctx, genai.Text(text))
	if err != nil {
		log.Printf("Error sending message to Gemini for user %s: %v", userID, err)
		SendResponse(nc, userID, "Lo siento, hubo un error al procesar tu solicitud con el modelo de IA. Intenta de nuevo.")
		return
	}

	// --- Inicio de la lógica para manejar múltiples tool calls ---
	var toolResponses []genai.Part // Usaremos esto para recolectar todas las respuestas de las herramientas
	var hasToolCalls bool = false

	for _, part := range resp.Candidates[0].Content.Parts {
		if tc, ok := part.(genai.FunctionCall); ok {
			hasToolCalls = true
			log.Printf("Gemini requested tool call: %s(%v)", tc.Name, tc.Args)

			if tc.Name == "create_calendar_event" {
				if summary, ok := tc.Args["summary"].(string); ok {
					log.Printf("DEBUG CALENDAR: summary from Gemini: %s", summary)
				}
				if startTime, ok := tc.Args["start_time"].(string); ok {
					log.Printf("DEBUG CALENDAR: start_time from Gemini: %s", startTime)
				}
				if endTime, ok := tc.Args["end_time"].(string); ok {
					log.Printf("DEBUG CALENDAR: end_time from Gemini: %s", endTime)
				}
				if timeZone, ok := tc.Args["time_zone"].(string); ok {
					log.Printf("DEBUG CALENDAR: time_zone from Gemini: %s", timeZone)
				}
			}

			toolOutput, toolErr := ExecuteToolCall(ctx, userID, session.OAuthTokens, tc.Name, tc.Args)
			if toolErr != nil {
				log.Printf("Error executing tool '%s' for user %s: %v", tc.Name, userID, toolErr)
				// Si hay un error, lo enviamos como una FunctionResponse con el error
				toolResponses = append(toolResponses, genai.FunctionResponse{
					Name: tc.Name,
					Response: map[string]interface{}{
						"error": toolErr.Error(),
					},
				})
				// Podrías decidir si detener el procesamiento de otras herramientas o continuar.
				// Por ahora, continuaremos para enviar todas las respuestas.
				continue // Pasar a la siguiente parte
			}
			log.Printf("Tool '%s' executed successfully. Output: %v", tc.Name, toolOutput)

			toolResponses = append(toolResponses, genai.FunctionResponse{
				Name: tc.Name,
				Response: map[string]interface{}{
					"result": toolOutput,
				},
			})
		} else if txt, ok := part.(genai.Text); ok {
			// Si hay texto directo de Gemini, lo manejamos inmediatamente si no hubo tool calls
			if !hasToolCalls {
				SendResponse(nc, userID, string(txt))
				return // Termina si solo es texto y no hay herramientas
			}
		}
	}

	if hasToolCalls {
		// Si se realizaron llamadas a herramientas, envía todas las respuestas de vuelta a Gemini
		respAfterTool, err := geminiChatSession.SendMessage(ctx, toolResponses...) // Usamos '...' para pasar los partes como argumentos variádicos
		if err != nil {
			log.Printf("Error sending tool outputs back to Gemini for user %s: %v", userID, err)
			SendResponse(nc, userID, "Lo siento, hubo un error al comunicar el resultado de las acciones.")
			return
		}

		// Obtén la respuesta final de Gemini después de las ejecuciones de herramientas
		for _, finalPart := range respAfterTool.Candidates[0].Content.Parts {
			if txt, ok := finalPart.(genai.Text); ok {
				SendResponse(nc, userID, string(txt))
				return
			}
		}
	}

	SendResponse(nc, userID, "Lo siento, no pude generar una respuesta clara.") // Fallback si no hubo texto ni tool calls o si el finalPart no fue texto.
}

// sendToolErrorToGemini sends an error response from a tool call back to Gemini
func sendToolErrorToGemini(ctx context.Context, sess *genai.ChatSession, toolName, errMsg string) {
	_, err := sess.SendMessage(ctx, genai.FunctionResponse{
		Name: toolName,
		Response: map[string]interface{}{
			"error": errMsg,
		},
	})
	if err != nil {
		log.Printf("Failed to send tool error response to Gemini: %v", err)
	}
}
