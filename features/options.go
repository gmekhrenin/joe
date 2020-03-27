/*
2019 © Postgres.ai
*/

// Package features provides Enterprise features and their mocks.
package features

// FlagProvider defines an interface to receive values of Enterprise application options.
type FlagProvider interface {
	ToOpts() EnterpriseOptions
}

// EnterpriseOptions describes Enterprise options of the application.
type EnterpriseOptions struct {
	QuotaLimit    uint
	QuotaInterval uint
	AuditEnabled  bool
}

// options contains extra flags for different editions of the application.
var flagProvider FlagProvider

// SetFlagProvider sets a flag provider for Enterprise options.
func SetFlagProvider(provider FlagProvider) {
	flagProvider = provider
}

// GetFlagProvider gets a flag provider of Enterprise options.
func GetFlagProvider() FlagProvider {
	return flagProvider
}
