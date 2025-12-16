// @title Tag Service API
// @version 1.0
// @description Microservice handles creation and retrieval of tags with validation and UUID management
//
// @host localhost:8098
// @BasePath /api/v1
//
// @tag.name Tags
// @tag.description "Tag operations: creating and receiving information"
package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	tags_create "tags/internal/app/handlers/tags/create"
	tags_info "tags/internal/app/handlers/tags/info"
	"tags/internal/app/middleware/logger"
	app_config "tags/internal/config/app-config"
	"tags/internal/lib/api/logger/sl"
	tag_service "tags/internal/services/tag-service"
	uuid_service "tags/internal/services/uuid-service"
	"tags/internal/storage/postgres"
	"time"

	_ "tags/docs"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	httpSwagger "github.com/swaggo/http-swagger"
)

const (
	envLocal = "local"
	envDev   = "dev"
	envProd  = "prod"
)

func main() {
	cfg := app_config.MustLoad()
	log := setupLogger(cfg.Env)
	log = log.With(slog.String("env", cfg.Env))

	log.Info("starting tags-app")
	log.Debug("logger debug mode enabled")

	storage, err := postgres.New(cfg)
	if err != nil {
		log.Error("failed to initialize storage", sl.Err(err))
		os.Exit(1)
	}

	tagService := tag_service.New(cfg.TagMeta.MaxTagLength)
	uuidService := uuid_service.New()

	router := chi.NewRouter()

	router.Use(middleware.RequestID)
	router.Use(middleware.Logger)
	router.Use(logger.New(log))
	router.Use(middleware.Recoverer)
	router.Use(middleware.URLFormat)

	router.Route("/api", func(r chi.Router) {
		r.Route("/v1", func(r chi.Router) {
			r.Post("/tags", tags_create.New(log, tagService, storage))
			r.Get("/tags/info", tags_info.New(log, storage, uuidService))
		})
	})

	router.Get("/swagger/*", httpSwagger.Handler(
		httpSwagger.URL("/swagger/doc.json"),
	))

	log.Info("starting server", slog.String("address", cfg.HTTPServer.Host+":"+cfg.HTTPServer.Port))

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	srv := &http.Server{
		Addr:         cfg.HTTPServer.Host + ":" + cfg.HTTPServer.Port,
		Handler:      router,
		ReadTimeout:  cfg.HTTPServer.Timeout,
		WriteTimeout: cfg.HTTPServer.Timeout,
		IdleTimeout:  cfg.HTTPServer.IdleTimeout,
	}

	serverErrors := make(chan error, 1)

	go func() {
		log.Info("server started")
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			serverErrors <- err
		}
	}()

	select {
	case <-ctx.Done():
		log.Info("shutdown signal received")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)

		defer cancel()

		if err := srv.Shutdown(shutdownCtx); err != nil {
			log.Error("failed to shutdown server gracefully", sl.Err(err))
			if err := srv.Close(); err != nil {
				log.Error("failed to close server", sl.Err(err))
			}
		}
		log.Info("server stopped gracefully")
	case err := <-serverErrors:
		log.Error("server failed to start", sl.Err(err))
		os.Exit(1)
	}
}

func setupLogger(env string) *slog.Logger {
	var log *slog.Logger

	switch env {
	case envLocal:
		log = slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))
	case envDev:
		log = slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))
	case envProd:
		log = slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	}

	return log
}
