/*
2019 © Postgres.ai
*/

package command

import (
	"context"

	"gitlab.com/postgres-ai/database-lab/pkg/client/dblabapi"
	"gitlab.com/postgres-ai/database-lab/pkg/log"

	"gitlab.com/postgres-ai/joe/pkg/connection"
	"gitlab.com/postgres-ai/joe/pkg/models"
	"gitlab.com/postgres-ai/joe/pkg/services/platform"
)

// ResetSession provides a command to reset a Database Lab session.
func ResetSession(ctx context.Context, cmd *platform.Command, msg *models.Message, dbLab *dblabapi.Client, cloneID string,
	msgSvc connection.Messenger) error {

	msg.AppendText("Resetting the state of the database...")
	msgSvc.UpdateText(msg)

	// TODO(anatoly): "zfs rollback" deletes newer snapshots. Users will be able
	// to jump across snapshots if we solve it.
	if err := dbLab.ResetClone(ctx, cloneID); err != nil {
		log.Err("Reset:", err)
		return err
	}

	result := "The state of the database has been reset."
	cmd.Response = result

	msg.AppendText(result)
	if err := msgSvc.UpdateText(msg); err != nil {
		log.Err("Reset:", err)
		return err
	}

	return nil
}
