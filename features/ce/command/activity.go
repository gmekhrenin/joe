/*
2019 © Postgres.ai
*/

// Package command provides assistant commands.
package command

import (
	"errors"

	"gitlab.com/postgres-ai/joe/features"
)

// ActivityCmd defines the activity command.
type ActivityCmd struct {
}

var _ features.Executor = (*ActivityCmd)(nil)

// Execute runs the activity command.
func (c *ActivityCmd) Execute() error {
	return errors.New("Enterprise feature. Not supported in CE version") // nolint:stylecheck
}
