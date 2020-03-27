/*
2019 © Postgres.ai
*/

// Package features provides Enterprise features and their mocks.
package features

// CmdBuilder provides a builder for Enterprise commands.
type CmdBuilder interface {
	BuildActivityCmd() Executor
	BuildTerminateCmd() Executor

	EnterpriseHelpMessenger
}

// EnterpriseHelpMessenger provides a help message for Enterprise commands.
type EnterpriseHelpMessenger interface {
	GetEnterpriseHelpMessage() string
}

// Executor describes a command interface.
type Executor interface {
	Execute() error
}
