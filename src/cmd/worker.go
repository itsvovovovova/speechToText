package main

import (
	"context"
	"database/sql"
	"log"
	"net/http"
	"os"
	"os/signal"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/prometheus/client_golang/prometheus/promhttp"

	"speechToText/src/config"
	"speechToText/src/consumer"
	"speechToText/src/db"
	"speechToText/src/metrics"
)

func runWorker(conn *sql.DB, store *db.Store) error {
	transcribe, err := consumer.NewTranscriber(config.CurrentConfig.Deepgram.ApiKey)
	if err != nil {
		return err
	}
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	var consuming atomic.Bool
	m := metrics.NewWorkerMetrics()
	mux := http.NewServeMux()
	mux.HandleFunc("/health", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })
	mux.HandleFunc("/ready", func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), time.Second)
		defer cancel()
		if !consuming.Load() || conn.PingContext(ctx) != nil {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusOK)
	})
	mux.Handle("/metrics", promhttp.Handler())
	port := os.Getenv("WORKER_PORT")
	if port == "" {
		port = "8081"
	}
	server := &http.Server{Addr: ":" + port, Handler: mux, ReadHeaderTimeout: 5 * time.Second}
	serverErr := make(chan error, 1)
	go func() { serverErr <- server.ListenAndServe() }()
	workerDone := make(chan struct{})
	go func() {
		defer close(workerDone)
		consumer.RunWorker(ctx, store, config.CurrentConfig.RabbitMQ.Url, consuming.Store, &m, transcribe)
	}()

	select {
	case <-ctx.Done():
	case err = <-serverErr:
	}
	stop()
	shutdown, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if shutdownErr := server.Shutdown(shutdown); shutdownErr != nil {
		log.Printf("worker HTTP shutdown: %v", shutdownErr)
	}
	<-workerDone
	return err
}
