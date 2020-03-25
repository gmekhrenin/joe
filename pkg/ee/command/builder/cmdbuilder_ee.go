// +build ee

/*
2019 © Postgres.ai
*/

// Package builder provides command builder for building the Enterprise commands.
package builder

import (
	"database/sql"

	"gitlab.com/postgres-ai/joe/pkg/ee"
	"gitlab.com/postgres-ai/joe/pkg/ee/command"

	"gitlab.com/postgres-ai/joe/pkg/bot/api"
	"gitlab.com/postgres-ai/joe/pkg/connection"
	"gitlab.com/postgres-ai/joe/pkg/models"
)

// EnterpriseBuilder defines an enterprise command builder.
type EnterpriseBuilder struct {
}

// NewBuilder creates a new enterprise command builder.
func NewBuilder() *EnterpriseBuilder {
	return &EnterpriseBuilder{}
}

// BuildActivityCmd builds a new activity command.
func (b *EnterpriseBuilder) BuildActivityCmd(apiCmd *api.ApiCommand, msg *models.Message, db *sql.DB,
	msgSvc connection.Messenger) ee.Executor {
	return command.NewActivityCmd(apiCmd, msg, db, msgSvc)
}

// BuildTerminateCmd builds a new activity command.
func (b *EnterpriseBuilder) BuildTerminateCmd(apiCmd *api.ApiCommand, msg *models.Message, db *sql.DB,
	msgSvc connection.Messenger) ee.Executor {
	return command.NewTerminateCmd(apiCmd, msg, db, msgSvc)
}
