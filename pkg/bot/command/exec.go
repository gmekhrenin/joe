/*
2019 © Postgres.ai
*/

package command

import (
	"database/sql"
	"fmt"
	"time"

	"github.com/pkg/errors"
	"gitlab.com/postgres-ai/database-lab/pkg/log"

	"gitlab.com/postgres-ai/joe/pkg/bot/api"
	"gitlab.com/postgres-ai/joe/pkg/bot/querier"
	"gitlab.com/postgres-ai/joe/pkg/chatapi"
	"gitlab.com/postgres-ai/joe/pkg/util"
)

// ExecCmd defines the exec command.
type ExecCmd struct {
	apiCommand *api.ApiCommand
	message    *chatapi.Message
	db         *sql.DB
}

// NewExec return a new exec command.
func NewExec(apiCmd *api.ApiCommand, msg *chatapi.Message, db *sql.DB) *ExecCmd {
	return &ExecCmd{
		apiCommand: apiCmd,
		message:    msg,
		db:         db,
	}
}

// Execute runs the exec command.
func (cmd ExecCmd) Execute() error {
	if cmd.apiCommand.Query == "" {
		return errors.New(MsgExecOptionReq)
	}

	start := time.Now()
	err := querier.DBExec(cmd.db, cmd.apiCommand.Query)
	elapsed := time.Since(start)
	if err != nil {
		log.Err("Exec:", err)
		return err
	}

	duration := util.DurationToString(elapsed)
	result := fmt.Sprintf("The query has been executed. Duration: %s", duration)
	cmd.apiCommand.Response = result

	if err = cmd.message.Append(result); err != nil {
		log.Err("Exec:", err)
		return err
	}

	return nil
}
