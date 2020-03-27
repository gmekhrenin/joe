// +build !ee

/*
2019 © Postgres.ai
*/

// Package ce provides mocks of Enterprise features.
package ce

import (
	"gitlab.com/postgres-ai/joe/features"
	"gitlab.com/postgres-ai/joe/features/ce/command/builder"
	"gitlab.com/postgres-ai/joe/features/ce/options"
)

// nolint:gochecknoinits
func init() {
	features.SetBuilder(builder.NewBuilder)
	features.SetFlagProvider(&options.Extra{})
}
