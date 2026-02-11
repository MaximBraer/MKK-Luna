package application

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"strings"
	"sync"

	"MKK-Luna/internal/api"
	"MKK-Luna/internal/config"
	"MKK-Luna/pkg/nethttp/runner"
)

type Application struct {
	cfg    *config.Config
	logger *slog.Logger
	router *api.Router

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

	return appErr
}

func (a *Application) initCoreComponents() error {
	if err := a.initConfig(); err != nil {
		return fmt.Errorf("initConfig(): %w", err)
	}

	a.initLogger()
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

func (a *Application) initPublicRouter(ctx context.Context) error {
	a.router = api.New(a.cfg, a.logger)

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
