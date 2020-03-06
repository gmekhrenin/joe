package assistant

import (
	"net/http"
)

type Assistant interface {
	Init() error
	Handlers() map[string]http.HandlerFunc
}
