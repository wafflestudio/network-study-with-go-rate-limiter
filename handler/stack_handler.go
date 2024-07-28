package handler

import (
	"net/http"

	"github.com/wafflestudio/network-study-with-go-rate-limiter/middleware"
)

var _ http.Handler = (*StackHandler)(nil)

type StackHandler struct {
	stackedHandler http.Handler
}

func NewStackHandler(middlewares []middleware.Middleware, handler http.Handler) *StackHandler {
	stackedHandler := handler
	for i := len(middlewares) - 1; i >= 0; i-- {
		mid := middlewares[i]
		stackedHandler = mid.ServeNext(stackedHandler)
	}

	return &StackHandler{stackedHandler}
}

// ServeHTTP implements http.Handler.
func (s *StackHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.stackedHandler.ServeHTTP(w, r)
}
