package api

import (
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	chiMiddleware "github.com/go-chi/chi/v5/middleware"

	middlewarex "MKK-Luna/internal/api/middleware"
	"MKK-Luna/internal/api/ratelimit"
	"MKK-Luna/internal/config"
	"MKK-Luna/internal/service"
)

type Router struct {
	*chi.Mux
	Server *http.Server
	logger *slog.Logger
	cfg    *config.Config
	auth   *service.AuthService
}

func New(cfg *config.Config, logger *slog.Logger, auth *service.AuthService) *Router {
	r := chi.NewRouter()

	r.Use(chiMiddleware.RequestID)
	r.Use(chiMiddleware.Recoverer)
	r.Use(middlewarex.Logger(logger))

	loginLimiter := ratelimit.New(cfg.Auth.LoginPerMin, time.Minute)
	refreshLimiter := ratelimit.New(cfg.Auth.RefreshPerMin, time.Minute)
	authHandler := NewAuthHandler(auth, loginLimiter, refreshLimiter)

	r.Get("/health", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})

	r.Route("/api/v1", func(r chi.Router) {
		r.Post("/register", authHandler.Register)
		r.Post("/login", authHandler.Login)
		r.Post("/refresh", authHandler.Refresh)
	})

	router := &Router{
		Mux:    r,
		logger: logger,
		cfg:    cfg,
		auth:   auth,
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
