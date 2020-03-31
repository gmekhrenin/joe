// +build ee

/*
2019 © Postgres.ai
*/

// Package builder provides command builder for building the Enterprise commands.
package builder

import (
	"github.com/jackc/pgx/v4"

	"gitlab.com/postgres-ai/joe/features/definition"
	"gitlab.com/postgres-ai/joe/features/edition/ee/command"
	"gitlab.com/postgres-ai/joe/pkg/bot/api"
	"gitlab.com/postgres-ai/joe/pkg/connection"
	"gitlab.com/postgres-ai/joe/pkg/models"
)

// EnterpriseBuilder defines an enterprise command builder.
type EnterpriseBuilder struct {
	apiCommand *api.ApiCommand
	message    *models.Message
	conn       *pgx.Conn
	messenger  connection.Messenger
}

var _ definition.CmdBuilder = (*EnterpriseBuilder)(nil)

// NewBuilder creates a new enterprise command builder.
func NewBuilder(apiCmd *api.ApiCommand, msg *models.Message, conn *pgx.Conn, msgSvc connection.Messenger) definition.CmdBuilder {
	return &EnterpriseBuilder{
		apiCommand: apiCmd,
		message:    msg,
		conn:       conn,
		messenger:  msgSvc,
	}
}

// BuildActivityCmd builds a new activity command.
func (b *EnterpriseBuilder) BuildActivityCmd() definition.Executor {
	return command.NewActivityCmd(b.apiCommand, b.message, b.conn, b.messenger)
}

// BuildTerminateCmd builds a new activity command.
func (b *EnterpriseBuilder) BuildTerminateCmd() definition.Executor {
	return command.NewTerminateCmd(b.apiCommand, b.message, b.conn, b.messenger)
}
