package main

import (
	"context"
	"flag"
	"log"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"time"

	"github.com/wafflestudio/network-study-with-go-rate-limiter/handler"
	"github.com/wafflestudio/network-study-with-go-rate-limiter/middleware"
	"github.com/wafflestudio/network-study-with-go-rate-limiter/middleware/logger_middleware"
	"github.com/wafflestudio/network-study-with-go-rate-limiter/middleware/logger_rate_limiter_middleware"
	"github.com/wafflestudio/network-study-with-go-rate-limiter/middleware/rate_limiter_middleware"
	rate_limiter "github.com/wafflestudio/network-study-with-go-rate-limiter/rate_limiter/leaky_bucket_rate_limiter"
	"github.com/wafflestudio/network-study-with-go-rate-limiter/rate_limiter/sliding_window_counter_rate_limiter"
)

var (
	addr = flag.String("addr", "localhost:8080", "define address of server")
)

func main() {
	// Logger middleware
	l := log.Default()
	l.SetFlags(log.Ltime | log.Lmicroseconds)
	logMiddleware := logger_middleware.NewLoggerMiddleware(l)

	// 별도의 로거 설정
	// tokenBucketLogger := createFileLogger("token_bucket.log")
	leakyBucketLogger := createFileLogger("leaky_bucket.log")
	leakyBucket2Logger := createFileLogger("leaky_bucket2.log")
	slidingWindowCounterLogger := createFileLogger("sliding_window_counter.log")

	// TODO: Add rate limiter
	// tokenBucketRateLimiter, err :=
	// rate_limiter.NewTokenBucketRateLimiterBuilder(5, nil)

	// .RefillGreedy(5, 10*time.Second, uriBasedKeyFunc, nil)
	// if err != nil {
	// 	log.Fatal("Invalid token bucket rate limiter")
	// }

	leakyBucketRateLimiter, err := rate_limiter.NewLeakyBucket(5, 10*time.Second, uriBasedKeyFunc, nil)
	if err != nil {
		log.Fatal("Invalid leaky bucket rate limiter")
	}

	leakyBucketRateLimiterMiddleware := rate_limiter_middleware.NewRateLimitMiddleware(leakyBucketRateLimiter)

	leakyBucket2RateLimiter, err := rate_limiter.NewLeakyBucket2(5, 10*time.Second, uriBasedKeyFunc, nil)
	if err != nil {
		log.Fatal("Invalid leaky bucket2 rate limiter")
	}

	leakyBucket2RateLimiterMiddleware := rate_limiter_middleware.NewRateLimitMiddleware(leakyBucket2RateLimiter)

	slidingWindowCounterRateLimiter, err := sliding_window_counter_rate_limiter.NewSlidingWindowCounter(3, 2, 5, uriBasedKeyFunc, nil)

	slidingWindowCounterRateLimiterMiddleware := rate_limiter_middleware.NewRateLimitMiddleware(slidingWindowCounterRateLimiter)

	leakyBucketLogging := &logger_rate_limiter_middleware.LoggerRateLimiterMiddleware{
		RateLimiterType: "LeakyBucket",
		Logger:          leakyBucketLogger,
	}

	leakyBucket2Logging := &logger_rate_limiter_middleware.LoggerRateLimiterMiddleware{
		RateLimiterType: "LeakyBucket2",
		Logger:          leakyBucket2Logger,
	}

	slidingWindowCounterLogging := &logger_rate_limiter_middleware.LoggerRateLimiterMiddleware{
		RateLimiterType: "SlidingWindowCounter",
		Logger:          slidingWindowCounterLogger,
	}

	leakyBucketHandler := handler.NewStackHandler(
		[]middleware.Middleware{leakyBucketLogging, leakyBucketRateLimiterMiddleware},
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Write([]byte("Leaky Bucket rate limiter response"))
		}),
	)

	leakyBucket2Handler := handler.NewStackHandler(
		[]middleware.Middleware{leakyBucket2Logging, leakyBucket2RateLimiterMiddleware},
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Write([]byte("Leaky Bucket2 rate limiter response"))
		}),
	)

	slidingWindowCouterHandler := handler.NewStackHandler(
		[]middleware.Middleware{slidingWindowCounterLogging, slidingWindowCounterRateLimiterMiddleware},
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Write([]byte("Sliding Window Counter rate limiter response"))
		}),
	)

	// Multiplexer handler
	mux := http.NewServeMux()
	mux.Handle("/leaky-bucket/", leakyBucketHandler)
	mux.Handle("/leaky-bucket2/", leakyBucket2Handler)
	mux.Handle("/sliding-window-counter/", slidingWindowCouterHandler)

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

func createFileLogger(filename string) *log.Logger {
	file, err := os.OpenFile(filename, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		log.Fatalf("Failed to open log file: %s", err)
	}
	return log.New(file, "", log.LstdFlags)
}
