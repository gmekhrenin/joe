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
	edition               = "CE"
	enterpriseHelpMessage = ""
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
