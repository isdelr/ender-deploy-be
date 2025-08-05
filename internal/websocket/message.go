package websocket

// Message defines the structure for websocket messages.
type Message struct {
	Action  string      `json:"action"`
	Payload interface{} `json:"payload"`
}
