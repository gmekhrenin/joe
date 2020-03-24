// +build !ee

/*
2019 © Postgres.ai
*/

// Package builder provides command builder for building the Enterprise commands.
package builder

import (
	"database/sql"

	"gitlab.com/postgres-ai/joe/pkg/bot/api"
	"gitlab.com/postgres-ai/joe/pkg/bot/command"
	"gitlab.com/postgres-ai/joe/pkg/connection"
	"gitlab.com/postgres-ai/joe/pkg/ee"
	"gitlab.com/postgres-ai/joe/pkg/models"
)

// Builder represents a builder for non enterprise activity command.
type Builder struct {
}

// NewBuilder creates a new activity builder.
func NewBuilder() *Builder {
	return &Builder{}
}

// BuildActivityCmd build a new Activity command.
func (builder *Builder) BuildActivityCmd(_ *api.ApiCommand, _ *models.Message, _ *sql.DB, _ connection.Messenger) ee.Executor {
	return &command.ActivityCmd{}
}
