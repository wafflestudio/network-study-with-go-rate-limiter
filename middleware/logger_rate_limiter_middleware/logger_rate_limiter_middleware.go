package logger_rate_limiter_middleware

import (
	"log"
	"net/http"
	"time"
)

type LoggerRateLimiterMiddleware struct {
	RateLimiterType string
	Logger          *log.Logger
}

func (lrlm *LoggerRateLimiterMiddleware) ServeNext(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()

		next.ServeHTTP(w, r)

		end := time.Now()
		duration := end.Sub(start)
		status := http.StatusOK
		if w.Header().Get("X-RateLimit-Exceeded") == "true" {
			status = http.StatusTooManyRequests
		}

		startFormatted := start.Format("2005-01-02 15:04:05.000")
		endFormatted := end.Format("2005-01-02 15:04:05.000")

		lrlm.Logger.Printf("%s, Status: %d, Req Time: %s, Resp Time: %s, Duration: %v, URI: %s\n",
			lrlm.RateLimiterType, status, startFormatted, endFormatted, duration, r.RequestURI)

	})
}
