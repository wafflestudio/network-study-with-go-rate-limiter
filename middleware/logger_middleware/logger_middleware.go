package logger_middleware

import (
	"bytes"
	"log"
	"net/http"

	"github.com/wafflestudio/network-study-with-go-rate-limiter/middleware"
)

var _ middleware.Middleware = (*LoggerMiddleware)(nil)
var _ http.ResponseWriter = (*bufferedResponseWriter)(nil)

type LoggerMiddleware struct {
	logger *log.Logger
}

func NewLoggerMiddleware(logger *log.Logger) *LoggerMiddleware {
	return &LoggerMiddleware{logger}
}

// ServeNext implements Middleware.
func (l *LoggerMiddleware) ServeNext(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		l.logger.Printf(
			"Request: [%s] %s - %s\n", r.Method, r.URL, r.RemoteAddr,
		)

		wrapedWriter := &bufferedResponseWriter{w: w}
		next.ServeHTTP(wrapedWriter, r)

		l.logger.Printf(
			"Response: [%d %s] %s",
			wrapedWriter.code,
			http.StatusText(wrapedWriter.code),
			wrapedWriter.body,
		)
	})
}

type bufferedResponseWriter struct {
	w    http.ResponseWriter
	code int
	body []byte
}

// Header implements http.ResponseWriter.
func (b *bufferedResponseWriter) Header() http.Header {
	return b.w.Header()
}

// Write implements http.ResponseWriter.
func (b *bufferedResponseWriter) Write(body []byte) (int, error) {
	b.body = bytes.Clone(body)
	return b.w.Write(body)
}

// WriteHeader implements http.ResponseWriter.
func (b *bufferedResponseWriter) WriteHeader(statusCode int) {
	b.code = statusCode
	b.w.WriteHeader(statusCode)
}
