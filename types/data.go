package types

// Message defines the Data of CloudEvent
type Message struct {
	// Msg holds the message from the event
	ID  int    `json:"id,omitempty,string"`
	Msg string `json:"msg,omitempty,string"`
}
