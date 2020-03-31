// +build ee

/*
2019 © Postgres.ai
*/

// Package command provides the Enterprise Edition commands.
package command

import (
	"database/sql"
	"strconv"
	"strings"

	"github.com/pkg/errors"

	"gitlab.com/postgres-ai/joe/features/definition"
	"gitlab.com/postgres-ai/joe/pkg/bot/api"
	"gitlab.com/postgres-ai/joe/pkg/bot/querier"
	"gitlab.com/postgres-ai/joe/pkg/connection"
	"gitlab.com/postgres-ai/joe/pkg/models"
)

// TerminateCaption contains caption for rendered tables.
const TerminateCaption = "*Terminate response:*\n"

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
	pid, err := strconv.Atoi(c.apiCommand.Query)
	if err != nil {
		return errors.Wrap(err, "invalid pid given")
	}

	query := "select pg_terminate_backend($1)"

	terminate, err := querier.DBQuery(c.db, query, pid)
	if err != nil {
		return errors.Wrap(err, "failed to make query")
	}

	tableString := &strings.Builder{}
	tableString.WriteString(TerminateCaption)

	querier.RenderTable(tableString, terminate)

	if err := c.messenger.UpdateText(c.message); err != nil {
		return errors.Wrap(err, "failed to publish message")
	}

	return nil
}
