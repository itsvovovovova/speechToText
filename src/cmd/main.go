package main

import (
	"context"
	"fmt"
	"log"
	"math"
	"net/http"
	"os"
	"os/signal"
	"speechToText/src/api"
	"speechToText/src/auth"
	"speechToText/src/cache"
	"speechToText/src/config"
	"speechToText/src/consumer"
	"speechToText/src/db"
	"speechToText/src/docs"
	appmetrics "speechToText/src/metrics"
	"speechToText/src/pkg/closer"
	"syscall"
	"time"

	httpSwagger "github.com/swaggo/http-swagger"

	"github.com/go-chi/chi/v5"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// @title Speech to Text API
// @version 1.0
// @description Speech to Text API
// @host localhost:8080
// @BasePath /
// @securityDefinitions.apikey ApiKeyAuth
// @in header
// @name Cookie
// @description Session cookie obtained from POST /login.
func main() {
	cl := closer.New()

	sqlDB, err := db.NewDB(config.CurrentConfig.Database)
	if err != nil {
		log.Fatalf("database init: %v", err)
	}
	cl.Add(sqlDB.Close)
	store := db.NewStore(sqlDB)
	if len(os.Args) == 2 && os.Args[1] == "worker" {
		err := runWorker(sqlDB, store)
		cl.Close()
		if err != nil {
			log.Fatal(err)
		}
		return
	}

	sessionProvider := cache.NewRedisSessionProvider(config.CurrentConfig.Redis.Host)
	cl.Add(sessionProvider.Close)
	sessionManager := cache.NewRedisSessionManager("session_id", sessionProvider, int64(math.Pow10(5)))

	handlers := api.NewHandlers(store, sessionManager)
	authMiddleware := auth.NewMiddleware(sessionManager)

	docs.SwaggerInfo.Title = "Speech to Text API"
	docs.SwaggerInfo.Description = "Speech to Text API with pagination"
	docs.SwaggerInfo.Version = "1.0"
	docs.SwaggerInfo.Host = "localhost:" + config.CurrentConfig.Server.Port
	docs.SwaggerInfo.BasePath = "/"

	appmetrics.RegisterTaskMetrics(sqlDB)
	m := appmetrics.NewMetrics()
	r := chi.NewRouter()
	r.Use(m.Middleware)

	r.Get("/swagger/*", httpSwagger.Handler(
		httpSwagger.URL(fmt.Sprintf("http://localhost:%s/swagger/doc.json", config.CurrentConfig.Server.Port)),
	))

	r.Get("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	r.Get("/ready", func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), time.Second)
		defer cancel()
		if sqlDB.PingContext(ctx) != nil || sessionProvider.Client.Ping(ctx).Err() != nil {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusOK)
	})
	r.Get("/metrics", promhttp.Handler().ServeHTTP)

	r.Post("/register", handlers.Register)
	r.Post("/login", handlers.Login)

	r.With(authMiddleware).Post("/logout", handlers.Logout)
	r.With(authMiddleware).Post("/audio", handlers.Audio)
	r.With(authMiddleware).Get("/status", handlers.Status)
	r.With(authMiddleware).Get("/result", handlers.Result)
	r.With(authMiddleware).Get("/tasks", handlers.Tasks)
	r.With(authMiddleware).Get("/tasks/{id}", handlers.GetTask)
	r.With(authMiddleware).Delete("/tasks/{id}", handlers.DeleteTask)

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()
	publisherDone := make(chan struct{})
	go func() {
		defer close(publisherDone)
		consumer.RunPublisher(ctx, store, config.CurrentConfig.RabbitMQ.Url)
	}()

	server := &http.Server{
		Addr:         ":" + config.CurrentConfig.Server.Port,
		Handler:      r,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	serverErr := make(chan error, 1)
	go func() {
		log.Printf("Server listening on port %s", config.CurrentConfig.Server.Port)
		serverErr <- server.ListenAndServe()
	}()

	var serveErr error
	select {
	case <-ctx.Done():
	case serveErr = <-serverErr:
	}

	cancel()
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer shutdownCancel()

	if err := server.Shutdown(shutdownCtx); err != nil {
		log.Printf("Error stopping server: %v\n", err)
	}

	<-publisherDone
	cl.Close()
	if serveErr != nil && serveErr != http.ErrServerClosed {
		log.Fatal(serveErr)
	}
}
