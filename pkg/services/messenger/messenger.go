package messenger

import (
	"gitlab.com/postgres-ai/joe/pkg/structs"
)

type Messenger interface {
	ValidateIncomingMessage(inputEvent *structs.IncomingMessage) error
	Publish(message *structs.Message) error    // post message: publish, publish ephemeral
	UpdateText(message *structs.Message) error // update message: append, replace
	UpdateStatus(message *structs.Message, status structs.MessageStatus) error
	// TODO: Finish(message *structs.Message) error // finish messaging: fail, ok
	Fail(message *structs.Message, text string) error // finish messaging: fail
	OK(message *structs.Message) error                // finish messaging: ok

	ArtifactLoader
}

type ArtifactLoader interface {
	AddArtifact(name string, result string, channelID string, messageID string) (artifactLink string, err error)
	DownloadArtifact(artifactURL string) (response []byte, err error)
}
