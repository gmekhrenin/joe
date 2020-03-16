/*
2019 © Postgres.ai
*/

package connection

import (
	"context"

	"gitlab.com/postgres-ai/joe/pkg/models"
	"gitlab.com/postgres-ai/joe/pkg/services/dblab"
)

// Assistant defines the interface of a Query Optimization assistant.
type Assistant interface {
	// Init defines the method to initialize the assistant.
	Init() error

	SetHandlerPrefix(prefix string)

	// CheckIdleSessions defines the method for checking user idle sessions and notification about them.
	CheckIdleSessions(context.Context)

	AddDBLabInstanceForChannel(channelID string, dbLabInstance *dblab.DBLabInstance)
}

type MessageProcessor interface {
	ProcessMessageEvent(incomingMessage models.IncomingMessage)
	ProcessAppMentionEvent(incomingMessage models.IncomingMessage)
	CheckIdleSessions(ctx context.Context)
}
