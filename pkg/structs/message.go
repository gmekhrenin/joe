package structs

import (
	"time"
)

// ChatAppendSeparator separates appended part of a message.
const ChatAppendSeparator = "\n\n"

const (
	StatusRunning = "running"
	StatusError   = "error"
	StatusOK      = "ok"
)

// IncomingMessage defines a standard representation of incoming events.
type IncomingMessage struct {
	SubType     string
	Text        string
	SnippetURL  string
	ChannelID   string
	ChannelType string
	UserID      string
	Timestamp   string
	ThreadID    string
}

// Message struct defines an output message.
type Message struct {
	MessageID   string
	MessageType string        // thread, ephemeral
	Status      MessageStatus // fail, success, wait: use for reactions as well.
	ChannelID   string
	ThreadID    string
	UserID      string
	CreatedAt   time.Time
	NotifyAt    time.Time
	Text        string // Used to accumulate message text to append new parts by edit.
}

// MessageStatus defines status of a message.
type MessageStatus string

func NewMessage(channelID string) *Message {
	return &Message{
		ChannelID: channelID,
	}
}

func (m *Message) SetText(text string) {
	m.Text = text
}

func (m *Message) AppendText(text string) {
	m.Text = m.Text + ChatAppendSeparator + text
}

func (m *Message) SetMessageType(messageType string) {
	m.MessageType = messageType
}

func (m *Message) SetStatus(status MessageStatus) {
	m.Status = status
}

func (m *Message) SetChatUserID(chatUserID string) {
	m.UserID = chatUserID
}

func (m *Message) SetLongRunningTimestamp(notificationTimeout time.Duration) error {
	if m.CreatedAt.IsZero() {
		return nil
	}

	// TODO (akartasov): check the logic.
	// Parse timestamp with microseconds.
	//parsedTimestamp, err := strconv.ParseInt(strings.Replace(m.MessageID, ".", "", -1), 10, 64)
	//if err != nil {
	//	return errors.Wrap(err, "failed to parse message timestamp")
	//}
	//
	//// Convert microseconds to time.
	//messageTimestamp := time.Unix(parsedTimestamp/1000000, 0)

	m.NotifyAt = m.CreatedAt.Add(notificationTimeout)

	return nil
}

func (m *Message) IsPublished() bool {
	return m.ChannelID != "" && m.MessageID != ""
}
