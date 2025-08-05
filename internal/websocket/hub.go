package websocket

import "github.com/rs/zerolog/log"

// Hub maintains the set of active clients and broadcasts messages to them.
type Hub struct {
	// Registered clients.
	clients map[*Client]bool

	// Inbound messages from the clients for global broadcast.
	Broadcast chan []byte

	// Register requests from the clients.
	Register chan *Client

	// Unregister requests from clients.
	Unregister chan *Client

	// A map of server IDs to a set of clients subscribed to it.
	subscriptions map[string]map[*Client]bool
}

// NewHub creates a new Hub.
func NewHub() *Hub {
	return &Hub{
		Broadcast:     make(chan []byte),
		Register:      make(chan *Client),
		Unregister:    make(chan *Client),
		clients:       make(map[*Client]bool),
		subscriptions: make(map[string]map[*Client]bool),
	}
}

// Run starts the Hub's message processing loop.
func (h *Hub) Run() {
	for {
		select {
		case client := <-h.Register:
			h.clients[client] = true
			log.Info().Int("total_clients", len(h.clients)).Msg("Client connected")
			// If client has a server ID on registration, subscribe them.
			if client.ServerID != "" {
				h.addSubscription(client, client.ServerID)
			}
		case client := <-h.Unregister:
			if _, ok := h.clients[client]; ok {
				// Remove from global clients and any subscriptions
				delete(h.clients, client)
				close(client.Send)
				h.removeSubscription(client)
				log.Info().Int("total_clients", len(h.clients)).Msg("Client disconnected")
			}
		case message := <-h.Broadcast:
			for client := range h.clients {
				select {
				case client.Send <- message:
				default:
					close(client.Send)
					delete(h.clients, client)
					h.removeSubscription(client)
				}
			}
		}
	}
}

// BroadcastTo sends a message to all clients subscribed to a specific server ID.
func (h *Hub) BroadcastTo(serverID string, message []byte) {
	if subs, ok := h.subscriptions[serverID]; ok {
		for client := range subs {
			select {
			case client.Send <- message:
			default:
				close(client.Send)
				delete(h.clients, client)
				delete(h.subscriptions[serverID], client)
			}
		}
	}
}

func (h *Hub) addSubscription(client *Client, serverID string) {
	if h.subscriptions[serverID] == nil {
		h.subscriptions[serverID] = make(map[*Client]bool)
	}
	h.subscriptions[serverID][client] = true
}

func (h *Hub) removeSubscription(client *Client) {
	for serverID, subs := range h.subscriptions {
		if _, ok := subs[client]; ok {
			delete(subs, client)
			if len(subs) == 0 {
				delete(h.subscriptions, serverID)
			}
		}
	}
}
