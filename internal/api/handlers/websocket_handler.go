package handlers

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/gorilla/websocket"
	"github.com/isdelr/ender-deploy-be/internal/services"
	ws "github.com/isdelr/ender-deploy-be/internal/websocket"
	"github.com/rs/zerolog/log"
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
		log.Error().Err(err).Msg("Failed to upgrade websocket connection")
		return
	}

	serverID := chi.URLParam(r, "id")
	client := ws.NewClient(h.hub, conn, serverID)
	h.hub.Register <- client

	// If the connection is for a specific server, start streaming its logs.
	// We pass the request's context, which gets cancelled when the client disconnects.
	if client.ServerID != "" {
		go h.serverService.StreamServerLogs(r.Context(), client.ServerID, client.Send)
	}

	go client.WritePump()
	go client.ReadPump(h.handleIncomingWSMessage)
}

// handleIncomingWSMessage processes messages received from a websocket client.
func (h *WebSocketHandler) handleIncomingWSMessage(client *ws.Client, message []byte) {
	var msg ws.Message
	if err := json.Unmarshal(message, &msg); err != nil {
		log.Error().Err(err).Bytes("message", message).Msg("Error decoding websocket message")
		return
	}

	switch msg.Action {
	case "send_command":
		if client.ServerID == "" {
			log.Warn().Msg("Client tried to send command without subscribing to a server")
			return
		}

		payload, ok := msg.Payload.(map[string]interface{})
		if !ok {
			log.Warn().Interface("payload", msg.Payload).Msg("Invalid payload for send_command")
			return
		}
		command, ok := payload["command"].(string)
		if !ok {
			log.Warn().Interface("payload", payload).Msg("Invalid command in payload")
			return
		}

		if _, err := h.serverService.SendCommandToServer(client.ServerID, command); err != nil {
			log.Error().Err(err).Str("server_id", client.ServerID).Str("command", command).Msg("Failed to send command to server")
			// Optionally, send an error message back to the client
		}
	default:
		log.Warn().Str("action", msg.Action).Msg("Unknown websocket action received")
	}
}
