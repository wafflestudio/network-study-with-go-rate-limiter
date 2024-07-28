package rate_limiter

import "net/http"

type RateLimiter interface {
	Key(r *http.Request) string
	Take(key string) bool
}
