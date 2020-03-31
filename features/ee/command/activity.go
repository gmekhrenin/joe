// +build ee

/*
2019 © Postgres.ai
*/

// Package command provides the Enterprise Edition commands.
package command

import (
	"database/sql"
	"strings"

	"github.com/pkg/errors"

	"gitlab.com/postgres-ai/joe/features/definition"
	"gitlab.com/postgres-ai/joe/pkg/bot/api"
	"gitlab.com/postgres-ai/joe/pkg/bot/querier"
	"gitlab.com/postgres-ai/joe/pkg/connection"
	"gitlab.com/postgres-ai/joe/pkg/models"
)

// ActivityCaption contains caption for rendered tables.
const ActivityCaption = "*Activity response:*\n"

// ActivityCmd defines the activity command.
type ActivityCmd struct {
	apiCommand *api.ApiCommand
	message    *models.Message
	db         *sql.DB
	messenger  connection.Messenger
}

var _ definition.Executor = (*ActivityCmd)(nil)

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
	query := `select pid, (case when (query != '' and length(query) > 30) then left(query, 30) || '...' else query end) as query, 
	application_name, state, wait_event, NOW()-query_start as duration 
	from pg_stat_activity 
	where pid <> pg_backend_pid();`

	tableString := &strings.Builder{}
	tableString.WriteString(ActivityCaption)

	activity, err := querier.DBQuery(c.db, query)
	if err != nil {
		return errors.Wrap(err, "failed to make query")
	}

	querier.RenderTable(tableString, activity)

	if err := c.messenger.UpdateText(c.message); err != nil {
		return errors.Wrap(err, "failed to publish message")
	}

	return nil
}
