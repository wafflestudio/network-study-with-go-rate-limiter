package main

import (
	"context"
	"flag"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"sync"
	"time"

	"github.com/wafflestudio/network-study-with-go-rate-limiter/handler"
	"github.com/wafflestudio/network-study-with-go-rate-limiter/middleware"
	"github.com/wafflestudio/network-study-with-go-rate-limiter/middleware/logger_middleware"
	"github.com/wafflestudio/network-study-with-go-rate-limiter/middleware/logger_rate_limiter_middleware"
	"github.com/wafflestudio/network-study-with-go-rate-limiter/middleware/rate_limiter_middleware"
	"github.com/wafflestudio/network-study-with-go-rate-limiter/rate_limiter"
	"github.com/wafflestudio/network-study-with-go-rate-limiter/rate_limiter/leaky_bucket2_rate_limiter"
	"github.com/wafflestudio/network-study-with-go-rate-limiter/rate_limiter/leaky_bucket_rate_limiter"
	"github.com/wafflestudio/network-study-with-go-rate-limiter/rate_limiter/sliding_window_counter_rate_limiter"
	"github.com/wafflestudio/network-study-with-go-rate-limiter/rate_limiter/token_bucket_rate_limiter"
)

var (
	addr = flag.String("addr", "localhost:8080", "define address of server")
)

type RateLimiterConfig struct {
	name     string
	path     string
	builder  func() (rate_limiter.RateLimiter, error)
	logFile  string
	response string
}

func main() {
	// Logger middleware
	l := log.Default()
	l.SetFlags(log.Ltime | log.Lmicroseconds)
	logMiddleware := logger_middleware.NewLoggerMiddleware(l)

	leakyBucketFileLogger := createFileLogger("logs", "leaky_bucket.log")
	leakyBucket2FileLogger := createFileLogger("logs", "leaky_bucket2.log")
	tokenBucketGreedyFileLogger := createFileLogger("logs", "token_bucket_greedy.log")
	tokenBucketIntervalFileLogger := createFileLogger("logs", "token_bucket_interval.log")

	slidingWindowCounterFileLogger := createFileLogger("logs", "sliding_window_counter.log")

	rateLimiterConfigs := []RateLimiterConfig{
		{
			name: "TokenBucket Greedy",
			path: "token-bucket-greedy",
			builder: func() (rate_limiter.RateLimiter, error) {
				return token_bucket_rate_limiter.NewTokenBucketRateLimiterBuilder(10, tokenBucketGreedyFileLogger).RefillGreedy(10*time.Second, 10).SetKeyFunc(uriBasedKeyFunc).Build()
			},
			logFile:  "token_bucket_greedy.log",
			response: "[Success] Token Bucket Greedy rate limiter response",
		},
		{
			name: "TokenBucket Interval",
			path: "token-bucket-interval",
			builder: func() (rate_limiter.RateLimiter, error) {
				return token_bucket_rate_limiter.NewTokenBucketRateLimiterBuilder(10, tokenBucketIntervalFileLogger).RefillInterval(10*time.Second, 10).SetKeyFunc(uriBasedKeyFunc).Build()
			},
			logFile:  "token_bucket_interval.log",
			response: "[Success] Token Bucket Interval rate limiter response",
		},
		{
			name: "LeakyBucket",
			path: "leaky-bucket",
			builder: func() (rate_limiter.RateLimiter, error) {
				return leaky_bucket_rate_limiter.NewLeakyBucket(5, 10*time.Second, uriBasedKeyFunc, leakyBucketFileLogger)
			},
			logFile:  "leaky_bucket.log",
			response: "[Success] Leaky Bucket rate limiter response",
		},
		{
			name: "LeakyBucket2",
			path: "leaky-bucket2",
			builder: func() (rate_limiter.RateLimiter, error) {
				return leaky_bucket2_rate_limiter.NewLeakyBucket2(5, 10*time.Second, uriBasedKeyFunc, leakyBucket2FileLogger)
			},
			logFile:  "leaky_bucket2.log",
			response: "[Success] Leaky Bucket2 rate limiter response",
		},
		{
			name: "SlidingWindowCounter",
			path: "sliding-window-counter",
			builder: func() (rate_limiter.RateLimiter, error) {
				return sliding_window_counter_rate_limiter.NewSlidingWindowCounter(5*time.Second, 2*time.Second, 5, uriBasedKeyFunc, slidingWindowCounterFileLogger)
			},
			logFile:  "sliding_window_counter.log",
			response: "[Success] Sliding Window Counter rate limiter response",
		},
	}
	mux := http.NewServeMux()

	for _, config := range rateLimiterConfigs {
		rateLimiter, err := config.builder()
		if err != nil {
			log.Fatalf("Invalid %s rate limiter: %v", config.name, err)
		}
		rateLimiterMiddleware := rate_limiter_middleware.NewRateLimitMiddleware(rateLimiter)
		rateLimiterLogger := createFileLogger("middleware_logs", config.logFile)
		rateLimiterLogging := &logger_rate_limiter_middleware.LoggerRateLimiterMiddleware{
			RateLimiterType: config.name,
			Logger:          rateLimiterLogger,
		}

		handler := handler.NewStackHandler(
			[]middleware.Middleware{rateLimiterLogging, rateLimiterMiddleware},
			http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Write([]byte(config.response))
			}),
		)

		mux.Handle("/"+config.path+"/", handler)
	}

	// Stacked Handler
	handler := handler.NewStackHandler(
		[]middleware.Middleware{
			logMiddleware,
		},
		mux,
	)

	// Server
	server := http.Server{
		Addr:              *addr,
		Handler:           handler,
		IdleTimeout:       10 * time.Second,
		ReadHeaderTimeout: 5 * time.Second,
	}

	// Graceful shutdown
	wg := new(sync.WaitGroup)
	go func() {
		c := make(chan os.Signal, 1)
		signal.Notify(c, os.Interrupt)

		for {
			if <-c == os.Interrupt {
				server.Shutdown(context.Background())
				log.Println("server is shutting down...")
				wg.Done()
			}
		}
	}()

	// Start server
	wg.Add(1)
	log.Println("server is starting at", *addr)
	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatalln("server error: ", err)
	}
	wg.Wait()
}

func uriBasedKeyFunc(r *http.Request) string {
	return r.RequestURI
}

func createFileLogger(folder, filename string) *log.Logger {
	// 폴더 경로를 절대 경로로 변환
	absFolder, err := filepath.Abs(folder)
	if err != nil {
		log.Fatalf("Failed to get absolute path: %s", err)
	}

	// 폴더가 존재하지 않으면 생성
	if _, err := os.Stat(absFolder); os.IsNotExist(err) {
		err := os.MkdirAll(absFolder, 0755)
		if err != nil {
			log.Fatalf("Failed to create log folder: %s", err)
		}
	}

	filePath := filepath.Join(absFolder, filename)

	file, err := os.OpenFile(filePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		log.Fatalf("Failed to open log file: %s", err)
	}
	return log.New(file, "", log.LstdFlags)
}
