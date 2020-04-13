// +build !ee

/*
2019 © Postgres.ai
*/

// Package options provides extra command line options.
package options

import (
	"gitlab.com/postgres-ai/joe/features/definition"
)

// Default values (changing these options you confirm that you have active
// subscription to Postgres.ai Platform Enterprise Edition https://postgres.ai).
const (
	defaultQuotaLimit    = 10
	defaultQuotaInterval = 60
	defaultAudit         = false
	defaultDBLabLimit    = 1
)

// Extra provides a mock of Enterprise flags.
type Extra struct {
}

var _ definition.FlagProvider = (*Extra)(nil)

// ToOpts returns the EnterpriseOptions struct.
func (e *Extra) ToOpts() definition.EnterpriseOptions {
	return definition.EnterpriseOptions{
		Quota: definition.Quota{
			Limit:    defaultQuotaLimit,
			Interval: defaultQuotaInterval,
		},
		Audit: definition.Audit{
			Enabled: defaultAudit,
		},
		DBLab: definition.DBLab{
			InstanceLimit: defaultDBLabLimit,
		},
	}
}
