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
	leakyBucketLogger := createFileLogger("leaky_bucket.log")
	leakyBucket2Logger := createFileLogger("leaky_bucket2.log")

	// TODO: Add rate limiter
	leakyBucketRateLimiter, err := rate_limiter.NewLeakyBucket(1, 10*time.Second, uriBasedKeyFunc, nil)
	if err != nil {
		log.Fatal("Invalid leaky bucket rate limiter")
	}

	leakyBucketRateLimiterMiddleware := rate_limiter_middleware.NewRateLimitMiddleware(leakyBucketRateLimiter)

	leakyBucket2RateLimiter, err := rate_limiter.NewLeakyBucket2(1, 10*time.Second, uriBasedKeyFunc, nil)
	if err != nil {
		log.Fatal("Invalid leaky bucket2 rate limiter")
	}

	leakyBucket2RateLimiterMiddleware := rate_limiter_middleware.NewRateLimitMiddleware(leakyBucket2RateLimiter)

	leakyBucketLogging := &logger_rate_limiter_middleware.LoggerRateLimiterMiddleware{
		RateLimiterType: "LeakyBucket",
		Logger:          leakyBucketLogger,
	}

	leakyBucket2Logging := &logger_rate_limiter_middleware.LoggerRateLimiterMiddleware{
		RateLimiterType: "LeakyBucket2",
		Logger:          leakyBucket2Logger,
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

	// Multiplexer handler
	mux := http.NewServeMux()
	mux.Handle("/leaky_bucket", leakyBucketHandler)
	mux.Handle("/leaky_bucket2", leakyBucket2Handler)

	mux.HandleFunc("/ping", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("pong"))
	})
	mux.HandleFunc("/tang", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("huru"))
	})

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
