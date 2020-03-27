// +build ee

/*
2019 © Postgres.ai
*/

// Package ee provides Enterprise features.
// Using this package you confirm that you have active subscription to Postgres.ai Platform Enterprise Edition https://postgres.ai.
package ee

import (
	"gitlab.com/postgres-ai/joe/features"
	"gitlab.com/postgres-ai/joe/features/ee/command/builder"
	"gitlab.com/postgres-ai/joe/features/ee/options"
)

// nolint:gochecknoinits
func init() {
	features.SetBuilder(builder.NewBuilder)
	features.SetFlagProvider(&options.Extra{})
}
