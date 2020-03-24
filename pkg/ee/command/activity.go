// +build ee

/*
2019 © Postgres.ai
*/

// Package command provides the Enterprise Edition commands.
package command

import (
	"fmt"
	"database/sql"

	"gitlab.com/postgres-ai/joe/pkg/bot/api"
	"gitlab.com/postgres-ai/joe/pkg/connection"
	"gitlab.com/postgres-ai/joe/pkg/models"
)

// ActivityCmd defines the activity command.
type ActivityCmd struct {
	apiCommand *api.ApiCommand
	message    *models.Message
	db         *sql.DB
	messenger  connection.Messenger
}

// NewActivityCmd return a new exec command.
func NewActivityCmd(apiCmd *api.ApiCommand, msg *models.Message, db *sql.DB, messengerSvc connection.Messenger) *ActivityCmd {
	return &ActivityCmd{
		apiCommand: apiCmd,
		message:    msg,
		db:         db,
		messenger:  messengerSvc,
	}
}

// Execute runs the activity command.
func (c *ActivityCmd) Execute() error {
	fmt.Println("EE")
	return nil
}
