package structs

import (
	"time"
)

// ChatAppendSeparator separates appended part of a message.
const ChatAppendSeparator = "\n\n"

// Message struct.
type Message struct {
	MessageID   string
	MessageType string        // thread, ephemeral
	Status      MessageStatus // fail, success, wait: use for reactions as well.
	ChannelID   string
	ThreadID    string
	UserID      string
	CreatedAt   time.Time
	Text        string // Used to accumulate message text to append new parts by edit.
}

// MessageStatus defines status of a message.
type MessageStatus string

func (m *Message) SetText(text string) {
	m.Text = text
}

func (m *Message) AppendText(text string) {
	m.Text = m.Text + ChatAppendSeparator + text
}

func (m *Message) SetStatus(status MessageStatus) {
	m.Status = status
}

func (m *Message) IsPublished() bool {
	return m.ChannelID != "" && m.MessageID != ""
}
