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
)

var addr = flag.String("addr", "localhost:8080", "define address of server")

func main() {
	// Logger middleware
	l := log.Default()
	l.SetFlags(log.Ltime | log.Lmicroseconds)
	logMiddleware := logger_middleware.NewLoggerMiddleware(l)

	// TODO: Add rate limiter
	// rateLimitMiddleware := middleware.NewRateLimitMiddleware(rateLimiter)

	// Multiplexer handler
	mux := http.NewServeMux()
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
