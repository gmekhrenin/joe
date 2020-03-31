// +build ee

/*
2019 © Postgres.ai
*/

// Package entertainer provides Enterprise entertainer service.
package entertainer

// Constants provide features description.
const (
	edition               = "Enterprise Edition"
	enterpriseHelpMessage = "• `activity` — show information related to the current activity of that process\n" +
		"• `terminate` — terminate a backend\n"
)

// Entertainer implements entertainer interface for the Enterprise edition.
type Entertainer struct {
}

var _ definition.EnterpriseHelpMessenger = (*Entertainer)(nil)

// New creates a new Entertainer for the Enterprise edition.
func New() *Entertainer {
	return &Entertainer{}
}

// GetEnterpriseHelpMessage provides description of enterprise features.
func (e Entertainer) GetEnterpriseHelpMessage() string {
	return enterpriseHelpMessage
}

// GetEdition provides the assistant edition.
func (e Entertainer) GetEdition() string {
	return edition
}
