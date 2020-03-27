/*
2019 © Postgres.ai
*/

// Package features provides Enterprise features and their mocks.
package features

import (
	"database/sql"

	"gitlab.com/postgres-ai/joe/pkg/bot/api"
	"gitlab.com/postgres-ai/joe/pkg/connection"
	"gitlab.com/postgres-ai/joe/pkg/models"
)

// CommandFactoryMethod defines a factory method to create Enterprise commands.
type CommandFactoryMethod func(*api.ApiCommand, *models.Message, *sql.DB, connection.Messenger) CmdBuilder

var commandBuilder CommandFactoryMethod

// SetBuilder sets Enterprise command builder on the application init.
func SetBuilder(cmdBuilder CommandFactoryMethod) {
	commandBuilder = cmdBuilder
}

// GetBuilder gets builder initialized Enterprise command builder.
func GetBuilder() CommandFactoryMethod {
	return commandBuilder
}
