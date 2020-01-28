/*
2019 © Postgres.ai
*/

package command

import (
	"github.com/pkg/errors"
	"gitlab.com/postgres-ai/database-lab/pkg/log"

	"gitlab.com/postgres-ai/joe/pkg/bot/api"
)

func Snapshot(apiCmd *api.ApiCommand) error {
	if apiCmd.Query == "" {
		return errors.New(MsgSnapshotOptionReq)
	}

	// TODO(akartasov): Add method to API.
	//err = b.Prov.CreateSnapshot(query)
	err := errors.New("an unsupported command for now")
	if err != nil {
		log.Err("Snapshot: ", err)

		return err
	}

	return nil
}
