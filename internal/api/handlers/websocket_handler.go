package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/gorilla/websocket"
	"github.com/isdelr/ender-deploy-be/internal/services"
	ws "github.com/isdelr/ender-deploy-be/internal/websocket"
	"github.com/rs/zerolog/log"
)

// WebSocketHandler handles upgrading HTTP connections to WebSocket connections.
type WebSocketHandler struct {
	hub              *ws.Hub
	serverService    services.ServerServiceProvider
	logStreamCancels map[*ws.Client]context.CancelFunc
	mu               sync.Mutex
}

// NewWebSocketHandler creates a new WebSocketHandler.
func NewWebSocketHandler(hub *ws.Hub, serverService services.ServerServiceProvider) *WebSocketHandler {
	return &WebSocketHandler{
		hub:              hub,
		serverService:    serverService,
		logStreamCancels: make(map[*ws.Client]context.CancelFunc),
	}
}

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin: func(r *http.Request) bool {
		// Allow all origins (consider tightening this in production).
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

	// Support both /ws/servers/{id} and /ws/global routes.
	serverID := chi.URLParam(r, "id")
	if serverID == "" {
		serverID = "global"
	}

	client := ws.NewClient(h.hub, conn, serverID)
	h.hub.Register <- client

	var wg sync.WaitGroup
	wg.Add(2)

	go func() {
		defer wg.Done()
		client.WritePump()
	}()
	go func() {
		defer wg.Done()
		client.ReadPump(h.handleIncomingWSMessage)
	}()

	// Cleanup on disconnect.
	go func() {
		wg.Wait()

		h.mu.Lock()
		if cancel, ok := h.logStreamCancels[client]; ok {
			log.Info().Str("client_id", client.ServerID).Msg("Client disconnected, cancelling associated log stream.")
			cancel()
			delete(h.logStreamCancels, client)
		}
		h.mu.Unlock()

		h.hub.Unregister <- client
	}()
}

// handleIncomingWSMessage processes messages received from a websocket client.
func (h *WebSocketHandler) handleIncomingWSMessage(client *ws.Client, message []byte) {
	var msg ws.Message
	if err := json.Unmarshal(message, &msg); err != nil {
		log.Error().Err(err).Bytes("message", message).Msg("Error decoding websocket message")
		return
	}

	switch msg.Action {
	case "subscribe_docker_logs":
		log.Info().Str("client_id", client.ServerID).Msg("Client subscribed to Docker logs")
		ctx, cancel := context.WithCancel(context.Background())

		h.mu.Lock()
		h.logStreamCancels[client] = cancel
		h.mu.Unlock()

		go h.serverService.StreamServerLogs(ctx, client.ServerID, client.Send)

	case "unsubscribe_docker_logs":
		log.Info().Str("client_id", client.ServerID).Msg("Client unsubscribed from Docker logs")
		h.mu.Lock()
		if cancel, ok := h.logStreamCancels[client]; ok {
			cancel()
			delete(h.logStreamCancels, client)
		}
		h.mu.Unlock()

	case "send_rcon_command":
		h.executeCommand(client, msg, "rcon")

	case "send_terminal_command":
		h.executeCommand(client, msg, "terminal")

	default:
		log.Warn().Str("action", msg.Action).Msg("Unknown websocket action received")
		client.Send <- ws.NewErrorMessage("Unknown action: " + msg.Action)
	}
}

// executeCommand is a helper to reduce code duplication for rcon and terminal commands.
func (h *WebSocketHandler) executeCommand(client *ws.Client, msg ws.Message, source string) {
	payload, ok := msg.Payload.(map[string]interface{})
	if !ok {
		client.Send <- ws.NewErrorMessage("Invalid payload for command")
		return
	}
	command, ok := payload["command"].(string)
	if !ok || command == "" {
		client.Send <- ws.NewErrorMessage("Invalid or empty command in payload")
		return
	}

	var response string
	var err error

	// Create a context with a timeout for the command execution.
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if source == "rcon" {
		response, err = h.serverService.SendCommandToServer(client.ServerID, command)
	} else if source == "terminal" {
		response, err = h.serverService.ExecuteTerminalCommand(ctx, client.ServerID, command)
	}

	if err != nil {
		log.Error().Err(err).Str("server_id", client.ServerID).Str("command", command).Msg("Failed to execute command")
		client.Send <- ws.NewErrorMessage(err.Error())
		return
	}

	responseMsg := ws.NewConsoleOutputMessage(source, command, response)
	client.Send <- responseMsg
}
