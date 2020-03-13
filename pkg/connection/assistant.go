/*
2019 © Postgres.ai
*/

package connection

import (
	"context"

	"gitlab.com/postgres-ai/joe/pkg/services/dblab"
)

// Assistant defines the interface of a Query Optimization assistant.
type Assistant interface {
	// Register defines the method to initialize the assistant.
	Register() error

	SetHandlerPrefix(prefix string)

	// CheckIdleSessions defines the method for checking user idle sessions and notification about them.
	CheckIdleSessions(context.Context)
}

type AssistantBuiilder interface {
	Build(dbLabInstance *dblab.DBLabInstance) Assistant
}
