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
	RefillInterval   time.Duration
	RefillTokens     int
	IsRefillInterval bool
	Capacity         int

	// Configs not open
	logger  *log.Logger
	keyFunc func(r *http.Request) string

	// Internal Structures
	bucketsLock sync.Mutex
	buckets     map[string]*bucket
}

type Builder struct {
	// Configs
	refillInterval   time.Duration
	refillTokens     int
	isRefillInterval bool
	capacity         int

	// Configs not open
	logger  *log.Logger
	keyFunc func(r *http.Request) string
}

func (b *Builder) RefillGreedy(refillInterval time.Duration, refillTokens int) *Builder {
	b.refillInterval = refillInterval
	b.refillTokens = refillTokens
	b.isRefillInterval = false
	return b
}

func (b *Builder) RefillInterval(refillInterval time.Duration, refillTokens int) *Builder {
	b.refillInterval = refillInterval
	b.refillTokens = refillTokens
	b.isRefillInterval = true
	return b
}

func (b *Builder) SetKeyFunc(keyFunc func(r *http.Request) string) *Builder {
	b.keyFunc = keyFunc
	return b
}

func (b *Builder) Build() (*TokenBucketRateLimiter, error) {
	if b.capacity <= 0 {
		return nil, errors.New("capacity should be greater than 0")
	}
	if b.refillTokens <= 0 {
		return nil, errors.New("refill tokens should be greater than 0")
	}
	return &TokenBucketRateLimiter{
		RefillInterval:   b.refillInterval,
		RefillTokens:     b.refillTokens,
		IsRefillInterval: b.isRefillInterval,
		Capacity:         b.capacity,
		logger:           b.logger,
		keyFunc:          b.keyFunc,
		bucketsLock:      sync.Mutex{},
		buckets:          make(map[string]*bucket),
	}, nil
}

// NewTokenBucketRateLimiter If keyFunc is nil, use remote address as key
func NewTokenBucketRateLimiterBuilder(
	capacity int,
	logger *log.Logger,
) *Builder {
	keyFunc := func(r *http.Request) string {
		return r.RemoteAddr
	}
	return &Builder{
		refillInterval:   time.Minute,
		refillTokens:     100,
		isRefillInterval: false,
		capacity:         capacity,
		logger:           logger,
		keyFunc:          keyFunc,
	}
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
					refillInterval:   s.RefillInterval,
					refillTokens:     s.RefillTokens,
					isRefillInterval: s.IsRefillInterval,
					capacity:         s.Capacity,
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
	refillInterval   time.Duration
	refillTokens     int
	isRefillInterval bool
	capacity         int
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
	elapsedFraction := float64(duration) / float64(refillInterval)
	refill := 0
	if s.ctx.isRefillInterval { // when interval
		refill = int(elapsedFraction) * refillTokens
	} else { // when greedy
		refill = int(elapsedFraction * float64(refillTokens))
	}

	s.tokens = min(capacity, t+refill)
	if t < s.tokens {
		s.lastRefillTime = currentTime
	}
}

func (s *State) Consume(tokens int) bool {
	fmt.Printf("try Consume: current tokens=%v, tokens to conusme=%v\n", s.tokens, tokens)
	if tokens > s.tokens {
		return false
	}
	s.tokens = max(0, s.tokens-tokens)
	return true
}
