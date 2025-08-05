package handlers

import (
	"log"
	"net/http"

	"github.com/gorilla/websocket"
	ws "github.com/isdelr/ender-deploy-be/internal/websocket"
)

// WebSocketHandler handles upgrading HTTP connections to WebSocket connections.
type WebSocketHandler struct {
	hub *ws.Hub
}

// NewWebSocketHandler creates a new WebSocketHandler.
func NewWebSocketHandler(hub *ws.Hub) *WebSocketHandler {
	return &WebSocketHandler{hub: hub}
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

	client := ws.NewClient(h.hub, conn)
	h.hub.Register <- client

	// Allow collection of memory referenced by the go-routines
	go client.WritePump()
	go client.ReadPump()
}
