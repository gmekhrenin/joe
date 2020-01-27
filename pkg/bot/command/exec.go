/*
2019 © Postgres.ai
*/

package command

import (
	"fmt"
	"time"

	"github.com/pkg/errors"
	"gitlab.com/postgres-ai/database-lab/pkg/log"

	"gitlab.com/postgres-ai/joe/pkg/bot/api"
	"gitlab.com/postgres-ai/joe/pkg/bot/querier"
	"gitlab.com/postgres-ai/joe/pkg/chatapi"
	"gitlab.com/postgres-ai/joe/pkg/util"
)

func Exec(apiCmd *api.ApiCommand, msg *chatapi.Message, connStr string) error {
	if apiCmd.Query == "" {
		return errors.New(MsgExecOptionReq)
	}

	start := time.Now()
	err := querier.DBExec(connStr, apiCmd.Query)
	elapsed := time.Since(start)
	if err != nil {
		log.Err("Exec:", err)
		return err
	}

	duration := util.DurationToString(elapsed)
	result := fmt.Sprintf("The query has been executed. Duration: %s", duration)
	apiCmd.Response = result

	if err = msg.Append(result); err != nil {
		log.Err("Exec:", err)
		return err
	}

	return nil
}
