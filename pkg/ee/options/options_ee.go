// +build ee

/*
2019 © Postgres.ai
*/

// Package options provides Enterprise command line options.
package options

// Enterprise features (changing these options you confirm that you have active subscription to Postgres.ai Platform Enterprise Edition https://postgres.ai).
// nolint:lll
type Enterprise struct {
	QuotaLimit    uint `long:"quota-limit" description:"limit request rates to up to 2x of this number" env:"QUOTA_LIMIT" default:"10"`
	QuotaInterval uint `long:"quota-interval" description:"a time interval (in seconds) to apply a quota-limit" env:"QUOTA_INTERVAL" default:"60"`
	AuditEnabled  bool `long:"audit-enabled" description:"enable logging of received commands" env:"AUDIT_ENABLED"`
}
