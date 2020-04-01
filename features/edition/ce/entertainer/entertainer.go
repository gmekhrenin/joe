/*
2019 © Postgres.ai
*/

// Package entertainer provides Enterprise entertainer service.
package entertainer

import (
	"gitlab.com/postgres-ai/joe/features/definition"
)

// Constants provide features description.
const (
	edition               = "Community Edition"
	enterpriseHelpMessage = "\n*Enterprise edition commands*:\n" +
		"• `activity` — show information related to the current activity of that process. Not supported in CE version\n" +
		"• `terminate` — terminate a backend. Not supported in CE version\n"
)

// Entertainer implements entertainer interface for the Community edition.
type Entertainer struct {
}

var _ definition.Entertainer = (*Entertainer)(nil)

// New creates a new Entertainer for the Community edition.
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
