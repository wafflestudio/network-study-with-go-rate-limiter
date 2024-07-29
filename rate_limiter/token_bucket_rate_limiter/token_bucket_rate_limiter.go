package rate_limiter

import (
	"errors"
	"fmt"
	"log"
	"net/http"
	"sync"
	"time"
)

type TokenBucketRateLimiter struct {
	// Configs
	RefillInterval time.Duration
	RefillTokens   int
	Capacity       int

	// Configs not open
	logger  *log.Logger
	keyFunc func(r *http.Request) string

	// Internal Structures
	bucketsLock sync.Mutex
	buckets     map[string]*bucket
}

// NewTokenBucketRateLimiter If keyFunc is nil, use remote address as key
func NewTokenBucketRateLimiter(
	refillInterval time.Duration,
	refillTokens int,
	capacity int,
	keyFunc func(r *http.Request) string,
	logger *log.Logger,
) (*TokenBucketRateLimiter, error) {
	if capacity <= 0 {
		return nil, errors.New("capacity should be greater than 0")
	}

	if refillTokens <= 0 {
		return nil, errors.New("refill tokens should be greater than 0")
	}

	if keyFunc == nil {
		keyFunc = func(r *http.Request) string {
			return r.RemoteAddr
		}
	}

	return &TokenBucketRateLimiter{
		RefillInterval: refillInterval,
		RefillTokens:   refillTokens,
		Capacity:       capacity,
		logger:         logger,
		keyFunc:        keyFunc,
		buckets:        map[string]*bucket{},
	}, nil
}

// Take implements RateLimiter.
func (s *TokenBucketRateLimiter) Take(key string) (accepted bool) {
	bucket := s.getBucket(key)
	return bucket.tryConsume(1)
}

// Key implements RateLimiter.
func (s *TokenBucketRateLimiter) Key(r *http.Request) string {
	return s.keyFunc(r)
}

func (s *TokenBucketRateLimiter) getBucket(key string) *bucket {
	if b, found := s.buckets[key]; found {
		return b
	}

	s.bucketsLock.Lock()
	defer s.bucketsLock.Unlock()

	b, found := s.buckets[key]
	if !found {
		b = &bucket{
			lock: sync.Mutex{},
			state: &State{
				lastRefillTime: time.Now(),
				tokens:         s.Capacity,
				ctx: &Context{
					refillInterval: s.RefillInterval,
					refillTokens:   s.RefillTokens,
					capacity:       s.Capacity,
				},
			},
		}
		s.buckets[key] = b
	}
	return b
}

type bucket struct {
	lock  sync.Mutex
	state *State
}

func (b *bucket) tryConsume(t int) bool {
	b.lock.Lock()
	defer b.lock.Unlock()

	state := b.state
	currentTime := time.Now()
	state.Refill(currentTime)

	return state.Consume(t)
}

type State struct {
	lastRefillTime time.Time
	tokens         int
	ctx            *Context
}

type Context struct {
	refillInterval time.Duration
	refillTokens   int
	capacity       int
}

func (s *State) Refill(currentTime time.Time) {
	l := s.lastRefillTime
	if currentTime.Before(l) {
		return
	}

	refillInterval := s.ctx.refillInterval
	refillTokens := s.ctx.refillTokens
	capacity := s.ctx.capacity
	t := s.tokens

	if capacity <= t {
		return
	}

	duration := currentTime.Sub(l)
	refillCount := int(duration.Nanoseconds() / refillInterval.Nanoseconds())
	if refillCount == 0 {
		return
	}
	t += refillCount * refillTokens

	s.tokens = min(capacity, t)
	s.lastRefillTime = currentTime
}

func (s *State) Consume(tokens int) bool {
	fmt.Printf("try Consume: current tokens=%v, tokens to conusme=%v\n", s.tokens, tokens)
	if tokens > s.tokens {
		return false
	}
	s.tokens = max(0, s.tokens-tokens)
	return true
}
