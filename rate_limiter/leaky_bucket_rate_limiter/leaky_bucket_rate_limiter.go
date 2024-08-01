package rate_limiter

import (
	"log"
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/wafflestudio/network-study-with-go-rate-limiter/rate_limiter"
)

var _ rate_limiter.RateLimiter = (*LeakyBucket)(nil)

type LeakyBucket struct {
	// Configs
	capacity int           // 버킷 최대 용량
	period   time.Duration // 용량만큼의 요청을 받아들일 기간

	// Configs not open
	keyFunc func(r *http.Request) string
	logger  *log.Logger

	// Internal Structures
	queues map[string]chan request
	mu     sync.Mutex
}

type request struct {
	done chan struct{}
}

func NewLeakyBucket(capacity int, period time.Duration, keyFunc func(r *http.Request) string, logger *log.Logger) (*LeakyBucket, error) {
	if keyFunc == nil {
		keyFunc = defaultKeyFunc
	}

	if logger == nil {
		logger = log.New(os.Stdout, "INFO: ", log.Ldate|log.Ltime|log.Lshortfile)
	}

	bucket := &LeakyBucket{
		capacity: capacity,
		period:   period,
		keyFunc:  keyFunc,
		logger:   logger,
		queues:   map[string]chan request{},
	}

	go bucket.startLeaking()

	return bucket, nil
}

func defaultKeyFunc(r *http.Request) string {
	return r.RemoteAddr
}

// Key implements rate_limiter.RateLimiter.
func (lb *LeakyBucket) Key(r *http.Request) string {
	return lb.keyFunc(r)
}

// Take implements rate_limiter.RateLimiter.
func (lb *LeakyBucket) Take(key string) (accepted bool) {
	queue := lb.getOrCreateQueue(key)
	req := request{make(chan struct{})}

	select {
	case queue <- req:
		<-req.done // 요청이 처리될 때까지 대기
		return true
	default:
		lb.logger.Printf("Request rejected for key: %s\n", key)
		return false
	}
}

func (lb *LeakyBucket) startLeaking() {

	rate := lb.period / time.Duration(lb.capacity)
	lb.logger.Printf("leaking rate: %s\n", rate)

	ticker := time.NewTicker(rate)
	defer ticker.Stop()

	// queue 자체가 map에 새로 생성될 수 있음
	for range ticker.C {
		for key, queue := range lb.queues {
			select {
			case req := <-queue:
				close(req.done)
				lb.logger.Printf("Processed request from queue: %s\n", key)
			default:
				lb.logger.Printf("empty bucket for queue: %s\n", key)
			}
		}
	}
}

func (lb *LeakyBucket) getOrCreateQueue(key string) chan request {
	q, found := lb.queues[key]
	if found {
		return q
	}
	lb.mu.Lock()
	defer lb.mu.Unlock()
	// key에 대응하는 queue가 없다면 새로 만들어줘야 함
	if !found {
		q = make(chan request, lb.capacity)
		lb.queues[key] = q
	}
	return q
}
