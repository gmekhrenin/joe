/*
2019 © Postgres.ai
*/

// Package command provides assistant commands.
package command

import (
	"errors"
)

// ActivityCmd defines the activity command.
type ActivityCmd struct {
}

// Execute runs the activity command.
func (c *ActivityCmd) Execute() error {
	return errors.New("Enterprise feature. Not supported in CE version") // nolint:stylecheck
}
