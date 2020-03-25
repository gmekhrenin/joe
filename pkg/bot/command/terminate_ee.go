/*
2019 © Postgres.ai
*/

// Package command provides assistant commands.
package command

// TerminateCmd defines the terminate command.
type TerminateCmd struct {
}

// Execute runs the terminate command.
func (c *TerminateCmd) Execute() error {
	return nil
}
