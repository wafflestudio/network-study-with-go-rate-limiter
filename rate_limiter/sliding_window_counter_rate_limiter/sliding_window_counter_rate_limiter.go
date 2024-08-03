package sliding_window_counter_rate_limiter

import (
	"errors"
	"fmt"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	rl "github.com/wafflestudio/network-study-with-go-rate-limiter/rate_limiter"
)

var _ rl.RateLimiter = (*SlidingWindowCounter)(nil)

type SlidingWindowCounter struct {
	// Configs
	Interval       time.Duration
	WindowInterval time.Duration
	Limit          int

	// Configs not open
	logger  *log.Logger
	keyFunc func(r *http.Request) string

	// Internal Structures
	windowsLock sync.Mutex
	windows     map[string]*window
}

// If keyFunc is nil, use remote address as key
func NewSlidingWindowCounter(
	interval time.Duration,
	windowInterval time.Duration,
	limit int,
	keyFunc func(r *http.Request) string,
	logger *log.Logger,
) (*SlidingWindowCounter, error) {
	if interval <= windowInterval {
		return nil, errors.New("interval should be greater than window interval")
	}

	if limit <= 0 {
		return nil, errors.New("limit should be greater than 0")
	}

	if keyFunc == nil {
		keyFunc = func(r *http.Request) string {
			return r.RemoteAddr
		}
	}

	return &SlidingWindowCounter{
		Interval:       interval,
		WindowInterval: windowInterval,
		Limit:          limit,
		logger:         logger,
		keyFunc:        keyFunc,
		windows:        map[string]*window{},
	}, nil
}

// Take implements RateLimiter.
func (s *SlidingWindowCounter) Take(key string) (accepted bool) {
	window := s.getWindow(key)

	now := time.Now()
	window.lock.Lock()
	defer window.lock.Unlock()

	// Calculate count, removable index
	count, removableIdx := s.calculateCountAndRemovableIndex(now, window)
	if s.logger != nil {
		s.logger.Printf(
			"key: %s\ncount: %f\nrecords: %+v",
			key, count, window.records,
		)
	}

	// Remove unneeded records
	window.records = window.records[removableIdx+1:]

	// early return when rate is limited
	if count >= float64(s.Limit) {
		return false
	}

	s.addCount(now, window)
	return true
}

// Key implements RateLimiter.
func (s *SlidingWindowCounter) Key(r *http.Request) string {
	return s.keyFunc(r)
}

// Calculate count, removable index
func (s *SlidingWindowCounter) calculateCountAndRemovableIndex(
	now time.Time, window *window,
) (count float64, removableIdx int) {
	lowerNow := now.Add(-1 * s.Interval)
	removableIdx = -1
	count = 0.0

	for i, r := range window.records {
		startsAt := r.endsAt.Add(-1 * s.WindowInterval)
		switch {
		case r.endsAt.Before(lowerNow) || r.endsAt.Equal(lowerNow): // out of range to lower part
			removableIdx = i
		case startsAt.After(now): // out of range to higher part
			break
		default: // apply ratio when lower window is split record
			var from time.Time
			if startsAt.Before(lowerNow) {
				from = lowerNow
			} else {
				from = startsAt
			}

			ratio := float64(r.endsAt.Sub(from)) / float64(s.WindowInterval)
			count += ratio * float64(r.count)
		}
	}

	return count, removableIdx
}

func (s *SlidingWindowCounter) addCount(now time.Time, window *window) {
	if ln := len(window.records); ln == 0 ||
		window.records[ln-1].endsAt.Before(now) {
		window.records = append(window.records, newRecord(now, s.WindowInterval))
	}

	window.records[len(window.records)-1].count++
}

func (s *SlidingWindowCounter) getWindow(key string) *window {
	if w, found := s.windows[key]; found {
		return w
	}

	s.windowsLock.Lock()
	defer s.windowsLock.Unlock()

	w, found := s.windows[key]
	if !found {
		w = &window{}
		s.windows[key] = w
	}
	return w
}

type window struct {
	lock    sync.Mutex
	records []*record
}

type record struct {
	endsAt time.Time
	count  int
}

func newRecord(now time.Time, interval time.Duration) *record {
	endsAt := now.Truncate(interval).Add(interval)
	return &record{
		endsAt: endsAt,
		count:  0,
	}
}

func (r *record) String() string {
	sb := new(strings.Builder)
	sb.WriteString("{\n")
	// sb.WriteString(fmt.Sprintf("\tendsAt: %s\n", r.endsAt))
	// sb.WriteString(fmt.Sprintf("\tcount: %d\n", r.count))
	sb.WriteString(fmt.Sprintf("\t%d, endsAt: %s\n", r.count, r.endsAt.Format("15:04:05.000")))
	sb.WriteString("}")
	return sb.String()
}
