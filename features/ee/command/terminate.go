// +build ee

/*
2019 © Postgres.ai
*/

// Package command provides the Enterprise Edition commands.
package command

import (
	"database/sql"
	"fmt"

	"gitlab.com/postgres-ai/joe/features/definition"
	"gitlab.com/postgres-ai/joe/pkg/bot/api"
	"gitlab.com/postgres-ai/joe/pkg/connection"
	"gitlab.com/postgres-ai/joe/pkg/models"
)

// TerminateCmd defines the terminate command.
type TerminateCmd struct {
	apiCommand *api.ApiCommand
	message    *models.Message
	db         *sql.DB
	messenger  connection.Messenger
}

var _ definition.Executor = (*TerminateCmd)(nil)

// NewTerminateCmd return a new terminate command.
func NewTerminateCmd(apiCmd *api.ApiCommand, msg *models.Message, db *sql.DB, messengerSvc connection.Messenger) *TerminateCmd {
	return &TerminateCmd{
		apiCommand: apiCmd,
		message:    msg,
		db:         db,
		messenger:  messengerSvc,
	}
}

// Execute runs the terminate command.
func (c *TerminateCmd) Execute() error {
	fmt.Println("EE")
	return nil
}
