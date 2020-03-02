/*
2019 © Postgres.ai
*/

package command

import (
	"database/sql"
	"strings"

	"github.com/pkg/errors"

	"gitlab.com/postgres-ai/joe/pkg/bot/api"
	"gitlab.com/postgres-ai/joe/pkg/bot/querier"
	"gitlab.com/postgres-ai/joe/pkg/chatapi"
)

// Hypo sub-commands
const (
	hypoCreate = "create"
	hypoDesc   = "desc"
	hypoDrop   = "drop"
	hypoReset  = "reset"
)

// HypoCmd defines a hypo command.
type HypoCmd struct {
	apiCommand *api.ApiCommand
	message    *chatapi.Message
	db         *sql.DB
}

// NewHypo creates a new Hypo command.
func NewHypo(apiCmd *api.ApiCommand, msg *chatapi.Message, db *sql.DB) *HypoCmd {
	return &HypoCmd{
		apiCommand: apiCmd,
		message:    msg,
		db:         db,
	}
}

// Execute runs the hypo command.
func (h *HypoCmd) Execute() error {
	hypoSub, commandTail := h.parseQuery()

	if err := h.initExtension(); err != nil {
		return errors.Wrap(err, "failed to init extension")
	}

	switch hypoSub {
	case hypoCreate:
		return h.create()

	case hypoDesc:
		return h.describe(commandTail)

	case hypoDrop:
		return h.drop(commandTail)

	case hypoReset:
		return h.reset()
	}

	return errors.New("invalid args given for the `hypo` command")
}

func (h *HypoCmd) parseQuery() (string, string) {
	parts := strings.SplitN(h.apiCommand.Query, " ", 2)

	hypoSubcommand := strings.ToLower(parts[0])

	if len(parts) < 2 {
		return hypoSubcommand, ""
	}

	return hypoSubcommand, parts[1]
}

func (h *HypoCmd) initExtension() error {
	return querier.DBExec(h.db, "CREATE EXTENSION IF NOT EXISTS hypopg;")
}

func (h *HypoCmd) create() error {
	query := "SELECT * FROM hypopg_create_index($1)"
	res, err := querier.DBQuery(h.db, query, h.apiCommand.Query)
	if err != nil {
		return errors.Wrap(err, "failed to run creation query")
	}

	tableString := querier.RenderTable(res)

	if err := h.message.Append(tableString.String()); err != nil {
		return errors.Wrap(err, "failed to publish message")
	}

	return nil
}

func (h *HypoCmd) describe(indexID string) error {
	query := "SELECT * FROM hypopg_list_indexes()"
	queryArgs := []interface{}{}

	if indexID != "" {
		query = `SELECT indexrelid, indexname, hypopg_get_indexdef(indexrelid), 
					pg_size_pretty(hypopg_relation_size(indexrelid)) 
					FROM hypopg_list_indexes() WHERE indexrelid = $1;`
		queryArgs = append(queryArgs, indexID)
	}

	res, err := querier.DBQuery(h.db, query, queryArgs...)
	if err != nil {
		return errors.Wrap(err, "failed to run description query")
	}

	tableString := querier.RenderTable(res)

	if err := h.message.Append(tableString.String()); err != nil {
		return errors.Wrap(err, "failed to publish message")
	}

	return nil
}

func (h *HypoCmd) drop(indexID string) error {
	if indexID == "" {
		return errors.Errorf("failed to drop a hypothetical index: indexrelid required")
	}

	query := "SELECT * FROM hypopg_drop_index($1);"
	res, err := querier.DBQuery(h.db, query, indexID)
	if err != nil {
		return errors.Wrap(err, "failed to drop index")
	}

	tableString := querier.RenderTable(res)

	if err := h.message.Append(tableString.String()); err != nil {
		return errors.Wrap(err, "failed to publish message")
	}

	return nil
}

func (h *HypoCmd) reset() error {
	res, err := querier.DBQuery(h.db, "SELECT * FROM hypopg_reset();")
	if err != nil {
		return errors.Wrap(err, "failed to reset indexes")
	}

	tableString := querier.RenderTable(res)

	if err := h.message.Append(tableString.String()); err != nil {
		return errors.Wrap(err, "failed to publish message")
	}

	return nil
}
