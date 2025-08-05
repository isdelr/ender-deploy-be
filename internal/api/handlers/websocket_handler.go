package handlers

import (
	"encoding/json"
	"log"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/gorilla/websocket"
	"github.com/isdelr/ender-deploy-be/internal/services"
	ws "github.com/isdelr/ender-deploy-be/internal/websocket"
)

// WebSocketHandler handles upgrading HTTP connections to WebSocket connections.
type WebSocketHandler struct {
	hub           *ws.Hub
	serverService services.ServerServiceProvider
}

// NewWebSocketHandler creates a new WebSocketHandler.
func NewWebSocketHandler(hub *ws.Hub, serverService services.ServerServiceProvider) *WebSocketHandler {
	return &WebSocketHandler{
		hub:           hub,
		serverService: serverService,
	}
}

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin: func(r *http.Request) bool {
		// Allow all connections for development.
		// In production, you should validate the origin.
		return true
	},
}

// Serve handles the WebSocket connection request.
func (h *WebSocketHandler) Serve(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("Failed to upgrade connection: %v", err)
		return
	}

	serverID := chi.URLParam(r, "id")
	client := ws.NewClient(h.hub, conn, serverID)
	h.hub.Register <- client

	// If the connection is for a specific server, start streaming its logs.
	if client.ServerID != "" {
		go h.serverService.StreamServerLogs(client.ServerID, client.Send)
	}

	go client.WritePump()
	go client.ReadPump(h.handleIncomingWSMessage)
}

// handleIncomingWSMessage processes messages received from a websocket client.
func (h *WebSocketHandler) handleIncomingWSMessage(client *ws.Client, message []byte) {
	var msg ws.Message
	if err := json.Unmarshal(message, &msg); err != nil {
		log.Printf("Error decoding websocket message: %v", err)
		return
	}

	switch msg.Action {
	case "send_command":
		if client.ServerID == "" {
			log.Println("Client tried to send command without subscribing to a server")
			return
		}

		payload, ok := msg.Payload.(map[string]interface{})
		if !ok {
			log.Printf("Invalid payload for send_command")
			return
		}
		command, ok := payload["command"].(string)
		if !ok {
			log.Printf("Invalid command in payload")
			return
		}

		if _, err := h.serverService.SendCommandToServer(client.ServerID, command); err != nil {
			log.Printf("Failed to send command to server %s: %v", client.ServerID, err)
			// Optionally, send an error message back to the client
		}
	default:
		log.Printf("Unknown websocket action: %s", msg.Action)
	}
}
