# Servidores MCP (Microservice Communication Protocol) y Chatbot Agente

Este proyecto implementa un sistema de agente inteligente conversacional capaz de interactuar con los servicios de Google (Calendar, Gmail, Contacts) a través de microservicios gRPC, utilizando Google Gemini para el procesamiento del lenguaje natural y un sistema de colas (NATS) para la comunicación asíncrona.

## Estructura del Proyecto

La estructura del proyecto está organizada en módulos ejecutables separados para facilitar el desarrollo, despliegue y gestión de dependencias.

your-project-root/
├── go.mod                      # Módulo Go principal del proyecto (para dependencias comunes si aplica)
├── go.sum
├── mcp_services.proto          # Definición de servicios gRPC (Protocol Buffers)
├── pb/                         # Directorio generado que contiene el código Go de los servicios gRPC
│   ├── mcp_services.pb.go
│   └── mcp_services_grpc.pb.go
├── mcp_server/                 # Módulo Go del Servidor MCP (gRPC)
│   ├── main.go                 # Contiene la función main() del servidor gRPC
│   └── credentials.json        # Credenciales de Google OAuth para la autenticación del servidor MCP
├── chatbot_agent/              # Módulo Go de la Capa del Chatbot (Cliente)
│   ├── go.mod                  # go.mod específico para el módulo del chatbot
│   ├── go.sum
│   ├── main.go                 # Función main() principal del chatbot (Gin, NATS, Gemini)
│   ├── gemini.go               # Lógica de interacción con Google Gemini (inicialización, tools)
│   ├── mcp_clients.go          # Cliente gRPC para comunicarse con el MCP Server y ejecutar Tool Calls
│   ├── nats_queue.go           # Funciones para la interacción con NATS (publicar/suscribir)
│   ├── session_manager.go      # Gestión en memoria de las sesiones de usuario (historial de chat, tokens)
│   ├── types.go                # Definiciones de estructuras de datos compartidas (ej. payloads de WhatsApp)
│   ├── .env                    # Variables de entorno para el chatbot (ej. GEMINI_API_KEY). ¡Ignorado por Git!
│   └── token.json              # Token de OAuth generado por el MCP Server para la autenticación en MCP
└── mcp_client_example/         # (Opcional) Módulo Go de un Cliente de Prueba directo del MCP Server
└── main.


## Configuración de Google Cloud Project (Para MCP Servers)

1.  **Crea un nuevo proyecto en Google Cloud Console:** [https://console.cloud.google.com/project](https://console.cloud.google.com/project)
2.  **Habilita las APIs:**
    * `Google Calendar API`
    * `Gmail API`
    * `Google People API`
3.  **Configura la pantalla de consentimiento de OAuth:**
    * Ve a "APIs y servicios" > "Pantalla de consentimiento de OAuth".
    * Selecciona "Externo" como tipo de usuario.
    * Completa la información básica.
    * **En la sección "Scopes", añade los siguientes scopes:**
        * `https://www.googleapis.com/auth/calendar.events`
        * `https://www.googleapis.com/auth/gmail.modify`
        * `https://www.googleapis.com/auth/contacts`
    * **Añade tu cuenta de Google como "Usuario de prueba"** en la sección "Usuarios de prueba" para poder testear la aplicación sin verificación completa.
    * Guarda la configuración.
4.  **Crea credenciales de ID de cliente de OAuth:**
    * Ve a "APIs y servicios" > "Credenciales".
    * Crea un "ID de cliente de OAuth" de tipo "Aplicación web".
    * **URI de redirección autorizado:** `http://localhost:8080/oauth2callback`
    * Descarga el archivo JSON y **renómbralo a `credentials.json`**. Colócalo en el directorio `mcp_server/`.

## Pasos para la Ejecución

### 1. Inicialización del Proyecto Go

1.  **Desde la raíz del proyecto (`your-project-root/`):**
    ```bash
    go mod tidy # Limpia y descarga dependencias para el módulo principal
    ```
2.  **Genera el código gRPC:**
    ```bash
    mkdir -p pb # Asegura que el directorio pb exista
    protoc --go_out=./pb --go_opt=paths=source_relative \
           --go-grpc_out=./pb --go-grpc_opt=paths=source_relative \
           mcp_services.proto
    ```
    Este comando leerá `mcp_services.proto` y generará los archivos `.go` en el directorio `pb/`.

3.  **Configura el módulo `chatbot_agent`:**
    ```bash
    cd chatbot_agent/
    go mod init [github.com/your-org/chatbot_agent](https://github.com/your-org/chatbot_agent) # Reemplaza con tu propio path o nombre
    go get [github.com/gin-gonic/gin](https://github.com/gin-gonic/gin) [github.com/joho/godotenv](https://github.com/joho/godotenv) [github.com/nats-io/nats.go](https://github.com/nats-io/nats.go) google.golang.org/grpc [github.com/google/generative-ai-go/genai](https://github.com/google/generative-ai-go/genai) google.golang.org/api/option
    # IMPORTANTE: Configura el 'replace' en chatbot_agent/go.mod si tu módulo pb está localmente
    # Ejemplo de replace en chatbot_agent/go.mod:
    # replace [github.com/your-org/mcp-services/pb](https://github.com/your-org/mcp-services/pb) => ../pb
    go mod tidy # Limpia y descarga dependencias para el módulo del chatbot
    cd .. # Vuelve a la raíz del proyecto
    ```
4.  **Mueve `credentials.json`:** Asegúrate de que tu `credentials.json` descargado esté en el directorio `mcp_server/`.

### 2. Iniciar el Servidor NATS (Usando Docker)

Abre una terminal **nueva** y ejecuta el servidor NATS:

```bash
docker run -p 4222:4222 -p 8222:8222 -p 6222:6222 nats -DV
Deja esta terminal abierta y el servidor NATS ejecutándose.

3. Iniciar los Servidores MCP (gRPC)
Abre otra terminal nueva (manteniendo NATS en su terminal) y ejecuta los servidores MCP:

Bash

go run ./mcp_server
Sigue las instrucciones en la consola para autorizar tu cuenta de Google (copia la URL en tu navegador y pega el código de verificación de vuelta en la terminal).
Una vez autorizado, el archivo token.json se creará en el directorio mcp_server/.
Copia este token.json al directorio chatbot_agent/. El chatbot lo necesitará para autenticar sus llamadas a los servidores MCP.
Deja esta terminal abierta y el servidor MCP ejecutándose.
4. Iniciar el Servidor del Chatbot (Client Face Layer)
Abre una tercera terminal nueva y ejecuta el servidor del chatbot.

Crea el archivo .env: Dentro del directorio chatbot_agent/, crea un archivo llamado .env y añade tu clave de API de Gemini:

Code snippet

# chatbot_agent/.env
GEMINI_API_KEY="TU_CLAVE_DE_API_GEMINI_AQUI"
¡REEMPLAZA TU_CLAVE_DE_API_GEMINI_AQUI con tu clave de API real de Gemini!

Corre el chatbot:

Bash

cd chatbot_agent/
go run .
Deja esta terminal abierta y el servidor del chatbot ejecutándose.

5. Probar el Chatbot (Simulación de WhatsApp)
Ahora puedes interactuar con el chatbot enviando solicitudes POST a su endpoint de webhook.

Ejemplo de cómo enviar un mensaje de prueba (usando curl):

Bash

curl -X POST http://localhost:8080/whatsapp/webhook \
     -H "Content-Type: application/json" \
     -d '{
    "object": "whatsapp_business_account",
    "entry": [{
        "id": "12345",
        "changes": [{
            "value": {
                "messaging_product": "whatsapp",
                "metadata": {
                    "display_phone_number_id": "123456789",
                    "phone_number_id": "987654321"
                },
                "contacts": [{
                    "profile": {
                        "name": "Usuario de Prueba"
                    },
                    "wa_id": "5491123456789"  // <--- ¡IMPORTANTE! Reemplaza con un ID de usuario de prueba (ej. tu número de teléfono con código de país sin '+')
                }],
                "messages": [{
                    "from": "5491123456789", // <--- ¡DEBE COINCIDIR CON wa_id ANTERIOR!
                    "id": "wamid.xxxxxxxxxxxxx",
                    "timestamp": "1678888888",
                    "text": {
                        "body": "Hola, ¿puedes listar mis próximos 3 eventos en el calendario?" // <--- ¡TU MENSAJE AQUÍ!
                    },
                    "type": "text"
                }]
            },
            "field": "messages"
        }]
    }]
}'
Para obtener la respuesta del chatbot (simulando que WhatsApp la recibiría):

Después de enviar el mensaje, el chatbot procesará la solicitud. Para ver la respuesta, necesitas consultar el endpoint de respuesta que simula el envío de vuelta a WhatsApp.

Bash

# Reemplaza 5491123456789 con el mismo wa_id/from que usaste en el POST anterior
curl http://localhost:8080/response/5491123456789
Esto esperará y te mostrará la respuesta del chatbot una vez que Gemini la genere y el sistema la publique en NATS.

Ejemplos de Interacciones con el Chatbot:
Puedes cambiar el body en el comando curl para probar estas frases:

"Hola, ¿puedes listar mis próximos 3 eventos en el calendario?"
"Quiero crear un evento. Se va a llamar 'Reunión de equipo'. La fecha es el 25 de mayo de 2025 de 10 AM a 11 AM, en la zona horaria de Buenos Aires."
"Envíale un correo a juan@example.com con el asunto 'Saludos' y el cuerpo 'Hola Juan, ¿cómo estás?'"
"¿Puedes mostrarme 2 de mis contactos?"
"Crea un nuevo contacto llamado 'Nuevo Amigo' con el email nuevo.amigo@example.com y el teléfono +1234567890."