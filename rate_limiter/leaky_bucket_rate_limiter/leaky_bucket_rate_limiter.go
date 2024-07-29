package rate_limiter

import (
	"log"
	"net/http"
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
	queueLock sync.Mutex
	queues    map[string]chan request
}

type request struct {
	done chan struct{}
}

func NewLeakyBucket(capacity int, period time.Duration, keyFunc func(r *http.Request) string, logger *log.Logger) (*LeakyBucket, error) {
	if keyFunc == nil {
		keyFunc = defaultKeyFunc
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
	queue := lb.getQueue(key)
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

	ticker := time.NewTicker(lb.period / time.Duration(lb.capacity))
	defer ticker.Stop()

	// queue 자체가 map에 새로 생성될 수 있음 (?)
	lb.queueLock.Lock()
	defer lb.queueLock.Unlock()
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

func (lb *LeakyBucket) getQueue(key string) chan request {
	if q, found := lb.queues[key]; found {
		return q
	}
	// key에 대응하는 queue가 없다면 새로 만들어줘야 함
	lb.queueLock.Lock()
	defer lb.queueLock.Unlock()

	q, found := lb.queues[key]
	if !found {
		q = make(chan request, lb.capacity)
		lb.queues[key] = q
	}
	return q
}
