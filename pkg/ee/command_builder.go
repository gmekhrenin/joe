/*
2019 © Postgres.ai
*/

package ee

import (
	"database/sql"

	"gitlab.com/postgres-ai/joe/pkg/bot/api"
	"gitlab.com/postgres-ai/joe/pkg/connection"
	"gitlab.com/postgres-ai/joe/pkg/models"
)

// Builder describes a builder for enterprise commands.
type Builder interface {
	BuildActivityCmd(apiCmd *api.ApiCommand, msg *models.Message, db *sql.DB, messengerSvc connection.Messenger) Executor
	BuildTerminateCmd(apiCmd *api.ApiCommand, msg *models.Message, db *sql.DB, messengerSvc connection.Messenger) Executor
	GetEnterpriseHelpMessage() string
}

// Executor describes a command interface.
type Executor interface {
	Execute() error
}
