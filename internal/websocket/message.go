package websocket

import "encoding/json"

// Message defines the structure for websocket messages.
type Message struct {
	Action  string      `json:"action"`
	Payload interface{} `json:"payload,omitempty"`
}

// ConsoleOutputPayload defines the structured payload for any console output.
type ConsoleOutputPayload struct {
	Source  string `json:"source"`            // "docker", "rcon", "terminal", "system"
	Command string `json:"command,omitempty"` // The original command sent by the user
	Line    string `json:"line"`
}

// --- Message Builder Functions ---

// NewConsoleOutputMessage creates a new 'console_output' message ready to be sent.
func NewConsoleOutputMessage(source, command, line string) []byte {
	msg := Message{
		Action: "console_output",
		Payload: ConsoleOutputPayload{
			Source:  source,
			Command: command,
			Line:    line,
		},
	}
	bytes, _ := json.Marshal(msg)
	return bytes
}

// NewErrorMessage creates a new 'console_output' message from the system to show an error.
func NewErrorMessage(line string) []byte {
	return NewConsoleOutputMessage("system", "", line)
}
