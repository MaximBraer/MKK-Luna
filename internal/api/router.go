package api

import (
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	middlewarex "MKK-Luna/internal/api/middleware"
	"MKK-Luna/internal/config"
)

type Router struct {
	*chi.Mux
	Server *http.Server
	logger *slog.Logger
	cfg    *config.Config
}

func New(cfg *config.Config, logger *slog.Logger) *Router {
	r := chi.NewRouter()

	r.Use(middleware.RequestID)
	r.Use(middleware.Recoverer)
	r.Use(middlewarex.Logger(logger))

	r.Get("/health", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})

	router := &Router{
		Mux:    r,
		logger: logger,
		cfg:    cfg,
	}

	router.Server = &http.Server{
		Addr:         cfg.HTTP.Addr,
		Handler:      router,
		ReadTimeout:  cfg.HTTP.ReadTimeout,
		WriteTimeout: cfg.HTTP.WriteTimeout,
		IdleTimeout:  cfg.HTTP.IdleTimeout,
	}

	return router
}
