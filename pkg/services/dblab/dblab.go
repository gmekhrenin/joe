package dblab

import (
	"gitlab.com/postgres-ai/database-lab/pkg/client/dblabapi"

	"gitlab.com/postgres-ai/joe/pkg/config"
)

// DBLabInstance contains a Database Lab client and its configuration.
type DBLabInstance struct {
	client *dblabapi.Client
	cfg    config.DBLabInstance
}

// NewDBLabInstance creates a new DBLabInstance.
func NewDBLabInstance(client *dblabapi.Client, cfg config.DBLabInstance) *DBLabInstance {
	return &DBLabInstance{client: client, cfg: cfg}
}

func (d DBLabInstance) Client() *dblabapi.Client {
	return d.client
}

func (d DBLabInstance) Config() config.DBLabInstance {
	return d.cfg
}
