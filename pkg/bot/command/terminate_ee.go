/*
2019 © Postgres.ai
*/

// Package command provides assistant commands.
package command

import (
	"github.com/pkg/errors"
)

// TerminateCmd defines the terminate command.
type TerminateCmd struct {
}

// Execute runs the terminate command.
func (c *TerminateCmd) Execute() error {
	return errors.New("Enterprise feature. Not supported in CE version") // nolint:stylecheck
}
