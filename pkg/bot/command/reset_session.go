/*
2019 © Postgres.ai
*/

package command

import (
	"context"

	"gitlab.com/postgres-ai/database-lab/pkg/client/dblabapi"
	"gitlab.com/postgres-ai/database-lab/pkg/log"

	"gitlab.com/postgres-ai/joe/pkg/bot/api"
	"gitlab.com/postgres-ai/joe/pkg/services/messenger"
	"gitlab.com/postgres-ai/joe/pkg/structs"
)

func ResetSession(ctx context.Context, apiCmd *api.ApiCommand, msg *structs.Message, dbLab *dblabapi.Client, cloneID string,
	msgSvc messenger.Messenger) error {

	msg.AppendText("Resetting the state of the database...")
	msgSvc.Append(msg)

	// TODO(anatoly): "zfs rollback" deletes newer snapshots. Users will be able
	// to jump across snapshots if we solve it.
	if err := dbLab.ResetClone(ctx, cloneID); err != nil {
		log.Err("Reset:", err)
		return err
	}

	result := "The state of the database has been reset."
	apiCmd.Response = result

	msg.AppendText(result)
	if err := msgSvc.Append(msg); err != nil {
		log.Err("Reset:", err)
		return err
	}

	return nil
}
