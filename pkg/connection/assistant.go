/*
2019 © Postgres.ai
*/

package connection

import (
	"context"
)

// Assistant defines the interface of a Query Optimization assistant.
type Assistant interface {
	Init() error
	CheckIdleSessions(context.Context)
}
