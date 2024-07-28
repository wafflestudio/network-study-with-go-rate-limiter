package middleware

import "net/http"

type Middleware interface {
	ServeNext(next http.Handler) http.Handler
}
