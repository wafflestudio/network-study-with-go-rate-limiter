package rate_limiter

import (
	"log"
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/wafflestudio/network-study-with-go-rate-limiter/rate_limiter"
)

var _ rate_limiter.RateLimiter = (*LeakyBucket2)(nil)

type LeakyBucket2 struct {
	// Configs
	capacity int           // 버킷 최대 용량
	period   time.Duration // 용량만큼의 요청을 받아들일 기간

	// Configs not open
	keyFunc func(r *http.Request) string
	logger  *log.Logger

	// Internal Structures
	queues          map[string]chan request2
	tickers         map[string]*time.Ticker
	lastLeakedTimes map[string]*time.Time
	mu              sync.Mutex
}

type request2 struct {
	done chan struct{}
}

func NewLeakyBucket2(capacity int, period time.Duration, keyFunc func(r *http.Request) string, logger *log.Logger) (*LeakyBucket2, error) {
	if keyFunc == nil {
		keyFunc = defaultKeyFunc
	}

	if logger == nil {
		logger = log.New(os.Stdout, "INFO: ", log.Ldate|log.Ltime|log.Lshortfile)
	}

	bucket := &LeakyBucket2{
		capacity:        capacity,
		period:          period,
		keyFunc:         keyFunc,
		logger:          logger,
		queues:          map[string]chan request2{},
		tickers:         map[string]*time.Ticker{},
		lastLeakedTimes: map[string]*time.Time{},
	}

	return bucket, nil
}

// Key implements rate_limiter.RateLimiter.
func (lb *LeakyBucket2) Key(r *http.Request) string {
	return lb.keyFunc(r)
}

// Take implements rate_limiter.RateLimiter.
func (lb *LeakyBucket2) Take(key string) (accepted bool) {
	queue := lb.getOrCreateQueue(key)

	rate := lb.period / time.Duration(lb.capacity)

	// 채널이 비어있는 경우
	if len(queue) == 0 {
		currentTimestamp := time.Now()
		lastLeaked, exists := lb.lastLeakedTimes[key]
		// 바로 요청 처리 가능한 시간이라면
		if !exists || currentTimestamp.After(lastLeaked.Add(rate)) {
			// lastLeakedTime 갱신
			lb.lastLeakedTimes[key] = &currentTimestamp
			// 대기 없이 바로 넘어가면 됨
			return true
		}
	}

	// 채널이 비어있지 않거나, 바로 요청 처리가 안되는 경우
	return lb.addRequest(queue, key)
}

func (lb *LeakyBucket2) addRequest(queue chan request2, key string) (accepted bool) {
	req := request2{make(chan struct{})}

	// 요청 추가 이후 대기
	select {
	case queue <- req:
		lb.startTickerForKey(key, queue)
		<-req.done // 요청이 처리될 때까지 대기
		currentTimestamp := time.Now()
		lb.lastLeakedTimes[key] = &currentTimestamp
		return true
	default:
		lb.logger.Printf("Request rejected for key: %s\n", key)
		return false
	}
}

func (lb *LeakyBucket2) startTickerForKey(key string, queue chan request2) {
	lb.mu.Lock()
	defer lb.mu.Unlock()

	if _, exists := lb.tickers[key]; exists {
		return
	}

	rate := lb.period / time.Duration(lb.capacity)
	lb.logger.Printf("leaking rate: %s\n", rate)

	var waitDuration time.Duration

	if lastLeaked, exists := lb.lastLeakedTimes[key]; exists {
		nextLeakTime := lastLeaked.Add(rate).Add(-1 * time.Second)
		waitDuration = time.Until(nextLeakTime)
		if waitDuration < 0 {
			waitDuration = 0
		}
		// 사실 이 경우는 넘어오면 안됨
	} else {
		waitDuration = 0
	}

	go func() {
		time.Sleep(waitDuration)
		lb.mu.Lock()
		if !lb.processRequest(queue, key) {
			lb.mu.Unlock()
			return
		}
		lb.mu.Unlock()

		ticker := time.NewTicker(rate)
		lb.tickers[key] = ticker

		for range ticker.C {
			lb.mu.Lock()
			if !lb.processRequest(queue, key) {
				ticker.Stop()
				delete(lb.tickers, key)
				lb.logger.Printf("empty bucket for queue: %s\n", key)
				lb.mu.Unlock()
				return
			}
		}
	}()

}

func (lb *LeakyBucket2) processRequest(queue chan request2, key string) bool {
	select {
	case req := <-queue:
		close(req.done)
		lb.logger.Printf("Processed request from queue: %s\n", key)
		return true
	default:
		return false
	}

}

func (lb *LeakyBucket2) getOrCreateQueue(key string) chan request2 {
	q, found := lb.queues[key]
	if found {
		return q
	}
	lb.mu.Lock()
	defer lb.mu.Unlock()
	// key에 대응하는 queue가 없다면 새로 만들어줘야 함
	if !found {
		q = make(chan request2, lb.capacity)
		lb.queues[key] = q
	}
	return q
}
