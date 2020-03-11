/*
2019 © Postgres.ai
*/

// package connection represents communication channels.
package connection

import (
	"gitlab.com/postgres-ai/joe/pkg/structs"
)

// Messenger defines the interface for communication with an assistant.
type Messenger interface {
	Publish(message *structs.Message) error    // post message: publish, publish ephemeral
	UpdateText(message *structs.Message) error // update message: append, replace
	UpdateStatus(message *structs.Message, status structs.MessageStatus) error
	// TODO: Finish(message *structs.Message) error // finish messaging: fail, ok
	Fail(message *structs.Message, text string) error // finish messaging: fail
	OK(message *structs.Message) error                // finish messaging: ok

	ArtifactLoader
}

// MessageValidator defines the interface for message validation.
type MessageValidator interface {
	Validate(inputEvent *structs.IncomingMessage) error
}

// MessageValidator defines the interface for artifacts management.
type ArtifactLoader interface {
	AddArtifact(name string, result string, channelID string, messageID string) (artifactLink string, err error)
	DownloadArtifact(artifactURL string) (response []byte, err error)
}
