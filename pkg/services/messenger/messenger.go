package messenger

import (
	"gitlab.com/postgres-ai/joe/pkg/structs"
)

type Messenger interface {
	Init() error
	Publish(message *structs.Message) error // post message: publish, publish ephemeral
	Append(message *structs.Message) error  // update message: append, replace
	UpdateStatus(message *structs.Message, status structs.MessageStatus) error
	//Finish(message *structs.Message) error // finish messaging: fail, ok
	Fail(message *structs.Message, text string) error // finish messaging: fail
	OK(message *structs.Message) error
	// finish messaging: ok

	//UploadFile("plan-wo-execution-text", explainResult, cmd.message.ChannelID, cmd.message.Timestamp)
	AddArtifact(string, string, string, string) (string, error)
}
