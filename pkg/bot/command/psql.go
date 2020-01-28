/*
2019 © Postgres.ai
*/

package command

import (
	"fmt"
	"strings"

	"github.com/pkg/errors"
	"gitlab.com/postgres-ai/database-lab/pkg/log"

	"gitlab.com/postgres-ai/joe/pkg/bot/api"
	"gitlab.com/postgres-ai/joe/pkg/chatapi"
	"gitlab.com/postgres-ai/joe/pkg/dblab"
	"gitlab.com/postgres-ai/joe/pkg/provision"
	"gitlab.com/postgres-ai/joe/pkg/util/text"
)

func PSQL(apiCmd *api.ApiCommand, msg *chatapi.Message, chat *chatapi.Chat, dbLabClone dblab.Clone, ch string) error {
	// See provision.runPsqlStrict for more comments.
	if strings.ContainsAny(apiCmd.Query, "\n;\\ ") {
		err := errors.New("query should not contain semicolons, new lines, spaces, and excess backslashes")
		log.Err(err)
		return err
	}

	psqlCmd := apiCmd.Command + " " + apiCmd.Query

	// TODO(akartasov): Keep psql for psql commands available to users - runPsqlStrict
	runner := provision.NewSQLRunner(dbLabClone, provision.LogsEnabledDefault)
	cmd, err := provision.RunPsqlStrict(runner, psqlCmd)
	if err != nil {
		log.Err(err)
		return err
	}

	apiCmd.Response = cmd

	cmdPreview, trnd := text.CutText(cmd, PlanSize, SeparatorPlan)

	if err = msg.Append(fmt.Sprintf("*Command output:*\n```%s```", cmdPreview)); err != nil {
		log.Err("Show psql output:", err)
		return err
	}

	fileCmd, err := chat.UploadFile("command", cmd, ch, msg.Timestamp)
	if err != nil {
		log.Err("File upload failed:", err)
		return err
	}

	detailsText := ""
	if trnd {
		detailsText = " " + CutText
	}

	if err = msg.Append(fmt.Sprintf("<%s|Full command output>%s\n", fileCmd.Permalink, detailsText)); err != nil {
		log.Err("File: ", err)
		return err
	}

	return nil
}
