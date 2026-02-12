package api

import (
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"
	chiMiddleware "github.com/go-chi/chi/v5/middleware"

	middlewarex "MKK-Luna/internal/api/middleware"
	"MKK-Luna/internal/config"
	"MKK-Luna/internal/domain/cache"
	"MKK-Luna/internal/domain/ratelimit"
	"MKK-Luna/internal/service"
)

type Router struct {
	*chi.Mux
	Server *http.Server
	logger *slog.Logger
	cfg    *config.Config
	auth   *service.AuthService
}

func New(
	cfg *config.Config,
	logger *slog.Logger,
	auth *service.AuthService,
	teams *service.TeamService,
	tasks *service.TaskService,
	taskCache cache.TaskCache,
	loginLimiter, refreshLimiter ratelimit.Limiter,
) *Router {
	r := chi.NewRouter()

	r.Use(chiMiddleware.RequestID)
	r.Use(chiMiddleware.Recoverer)
	r.Use(middlewarex.Logger(logger))

	authHandler := NewAuthHandler(auth, loginLimiter, refreshLimiter)
	teamHandler := NewTeamHandler(teams)
	taskHandler := NewTaskHandler(tasks, teams, taskCache)
	commentHandler := NewCommentHandler(tasks)

	r.Get("/health", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})

	r.Route("/api/v1", func(r chi.Router) {
		r.Post("/register", authHandler.Register)
		r.Post("/login", authHandler.Login)
		r.Post("/refresh", authHandler.Refresh)

		r.Group(func(r chi.Router) {
			r.Use(middlewarex.AuthMiddleware(auth))

			r.Post("/teams", teamHandler.Create)
			r.Get("/teams", teamHandler.List)
			r.Post("/teams/{id}/invite", teamHandler.Invite)

			r.Post("/tasks", taskHandler.Create)
			r.Get("/tasks", taskHandler.List)
			r.Get("/tasks/{id}", taskHandler.Get)
			r.Patch("/tasks/{id}", taskHandler.Update)
			r.Delete("/tasks/{id}", taskHandler.Delete)

			r.Post("/tasks/{id}/comments", commentHandler.Create)
			r.Get("/tasks/{id}/comments", commentHandler.ListByTask)
			r.Patch("/comments/{id}", commentHandler.Update)
			r.Delete("/comments/{id}", commentHandler.Delete)
		})
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
