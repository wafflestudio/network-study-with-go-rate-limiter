package rate_limiter_middleware

import (
	"net/http"

	"github.com/wafflestudio/network-study-with-go-rate-limiter/rate_limiter"
)

type RateLimitMiddleware struct {
	rate_limiter.RateLimiter
}

func NewRateLimitMiddleware(rateLimiter rate_limiter.RateLimiter) *RateLimitMiddleware {
	return &RateLimitMiddleware{rateLimiter}
}

func (rm *RateLimitMiddleware) ServeNext(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if rm.Take(rm.Key(r)) {
			next.ServeHTTP(w, r)
		} else {
			w.WriteHeader(http.StatusTooManyRequests)
			w.Write([]byte("Rate limit exceeded"))
		}
	})
}
