package messenger

import (
	"github.com/nlopes/slack/slackevents"
	"gitlab.com/postgres-ai/database-lab/pkg/client/dblabapi"

	"gitlab.com/postgres-ai/joe/pkg/structs"
)

type Messenger interface {
	Init() error
	Publish(message *structs.Message) error // post message: publish, publish ephemeral
	Append(message *structs.Message) error  // update message: append, replace
	UpdateStatus(message *structs.Message, status structs.MessageStatus) error
	//Finish(message *structs.Message) error // finish messaging: fail, ok
	Fail(message *structs.Message, text string) error // finish messaging: fail
	OK(message *structs.Message) error                // finish messaging: ok
}

type ProcessingService struct {
	//msgValidator    MessageEventValidator
	Messenger Messenger
	DBLab     *dblabapi.Client
	//PlatformManager
	//UserManager
	//Auditor
	//Limiter
}


func (s *ProcessingService) ProcessMessageEvent(ev *slackevents.MessageEvent) {
	// TODO(akartasov): Implement.
}
