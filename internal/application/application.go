package application

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"strings"
	"sync"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/redis/go-redis/v9"

	"MKK-Luna/internal/api"
	"MKK-Luna/internal/config"
	drl "MKK-Luna/internal/domain/ratelimit"
	"MKK-Luna/internal/infra/cache"
	rl "MKK-Luna/internal/infra/ratelimit"
	redisinfra "MKK-Luna/internal/infra/redis"
	"MKK-Luna/internal/repository"
	"MKK-Luna/internal/service"
	"MKK-Luna/pkg/nethttp/runner"
)

type Application struct {
	cfg            *config.Config
	logger         *slog.Logger
	router         *api.Router
	db             *sqlx.DB
	auth           *service.AuthService
	teamSvc        *service.TeamService
	taskSvc        *service.TaskService
	redis          *redis.Client
	loginLimiter   drl.Limiter
	refreshLimiter drl.Limiter
	taskCache      *cache.TaskCache

	errChan chan error
	wg      sync.WaitGroup
	ready   bool
}

func New() *Application {
	return &Application{errChan: make(chan error)}
}

func (a *Application) Ready() bool {
	return a.ready
}

func (a *Application) Start(ctx context.Context, build string) error {
	if err := a.initCoreComponents(); err != nil {
		return fmt.Errorf("initCoreComponents(): %w", err)
	}

	if err := a.initPublicRouter(ctx); err != nil {
		return fmt.Errorf("initPublicRouter(): %w", err)
	}

	a.logger.Info("application started", slog.String("build", build))
	a.ready = true
	return nil
}

func (a *Application) Wait(ctx context.Context, cancel context.CancelFunc) error {
	var appErr error

	errWg := sync.WaitGroup{}
	errWg.Add(1)

	go func() {
		defer errWg.Done()
		for err := range a.errChan {
			cancel()
			if err != nil {
				a.logger.Error("error in Wait", slog.String("error", err.Error()))
				appErr = err
			}
		}
	}()

	<-ctx.Done()
	a.wg.Wait()
	close(a.errChan)
	errWg.Wait()

	if a.db != nil {
		_ = a.db.Close()
	}

	return appErr
}

func (a *Application) initCoreComponents() error {
	if err := a.initConfig(); err != nil {
		return fmt.Errorf("initConfig(): %w", err)
	}

	a.initLogger()

	if err := a.initDB(); err != nil {
		return fmt.Errorf("initDB(): %w", err)
	}

	if err := a.initRedis(); err != nil {
		return fmt.Errorf("initRedis(): %w", err)
	}

	if err := a.initServices(); err != nil {
		return fmt.Errorf("initServices(): %w", err)
	}
	return nil
}

func (a *Application) initConfig() error {
	cfg, err := config.New()
	if err != nil {
		return err
	}
	a.cfg = cfg
	return nil
}

func (a *Application) initLogger() {
	a.logger = NewLogger(a.cfg.Log.LevelStr)
}

func (a *Application) initDB() error {
	db, err := repository.NewMySQL(a.cfg.MySQL)
	if err != nil {
		return err
	}
	a.db = db
	return nil
}

func (a *Application) initRedis() error {
	client := redisinfra.New(a.cfg.Redis)
	ok := client.Ping(context.Background(), a.logger)
	if ok {
		a.redis = client.Redis
	}

	window := time.Duration(a.cfg.RateLimit.WindowSeconds) * time.Second
	if window <= 0 {
		window = 60 * time.Second
	}

	fallbackLogin := rl.NewMemory(a.cfg.Auth.LoginPerMin, window)
	fallbackRefresh := rl.NewMemory(a.cfg.Auth.RefreshPerMin, window)

	if !a.cfg.RateLimit.Enabled {
		a.loginLimiter = rl.NewMemory(0, window)
		a.refreshLimiter = rl.NewMemory(0, window)
	} else if a.redis != nil {
		a.loginLimiter = rl.NewRedis(a.redis, a.cfg.Auth.LoginPerMin, window, fallbackLogin, a.logger)
		a.refreshLimiter = rl.NewRedis(a.redis, a.cfg.Auth.RefreshPerMin, window, fallbackRefresh, a.logger)
	} else {
		a.loginLimiter = fallbackLogin
		a.refreshLimiter = fallbackRefresh
	}

	a.taskCache = cache.NewTaskCache(a.redis, a.cfg.Cache.TaskCacheTTL, a.cfg.Cache.Enabled, a.logger)
	return nil
}

func (a *Application) initServices() error {
	userRepo := repository.NewUserRepository(a.db)
	teamRepo := repository.NewTeamRepository(a.db)
	memberRepo := repository.NewTeamMemberRepository(a.db)
	taskRepo := repository.NewTaskRepository(a.db)
	commentRepo := repository.NewTaskCommentRepository(a.db)
	historyRepo := repository.NewTaskHistoryRepository(a.db)

	sessionRepo := repository.NewSessionRepository(a.db)
	authSvc, err := service.NewAuthService(userRepo, sessionRepo, *a.cfg, a.logger)
	if err != nil {
		return err
	}
	a.auth = authSvc
	a.teamSvc = service.NewTeamService(a.db, teamRepo, memberRepo, userRepo)
	a.taskSvc = service.NewTaskService(a.db, taskRepo, teamRepo, memberRepo, commentRepo, historyRepo)
	return nil
}

func (a *Application) initPublicRouter(ctx context.Context) error {
	if a.auth == nil {
		return fmt.Errorf("auth service is nil")
	}
	if a.teamSvc == nil || a.taskSvc == nil {
		return fmt.Errorf("services are nil")
	}
	a.router = api.New(a.cfg, a.logger, a.auth, a.teamSvc, a.taskSvc, a.taskCache, a.loginLimiter, a.refreshLimiter)

	port, err := parsePort(a.cfg.HTTP.Addr)
	if err != nil {
		return err
	}

	if err := runner.RunServer(ctx, a.router.Server, port, a.errChan, &a.wg); err != nil {
		return err
	}

	return nil
}

func parsePort(addr string) (string, error) {
	if strings.HasPrefix(addr, ":") {
		return strings.TrimPrefix(addr, ":"), nil
	}
	_, port, err := net.SplitHostPort(addr)
	if err != nil {
		return "", fmt.Errorf("invalid http addr: %w", err)
	}
	return port, nil
}
