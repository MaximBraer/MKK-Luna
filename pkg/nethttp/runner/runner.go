package runner

//go:generate mockgen -destination=runner_mock.go -source=runner.go -package=runner

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"sync"
	"time"
)

type Server interface {
	Serve(listener net.Listener) error
	Shutdown(ctx context.Context) error
}

func RunServer(
	ctx context.Context,
	server Server,
	port string,
	errChan chan<- error,
	wgr *sync.WaitGroup,
	shutdownTimeout time.Duration,
) error {
	return runServer(ctx, server, port, errChan, wgr, net.Listen, shutdownTimeout)
}

func runServer(
	ctx context.Context,
	server Server,
	port string,
	errChan chan<- error,
	wgr *sync.WaitGroup,
	listen func(string, string) (net.Listener, error),
	shutdownTimeout time.Duration,
) error {
	listener, err := listen("tcp4", ":"+port)
	if err != nil {
		return fmt.Errorf("can't listen tcp port %s: %w", port, err)
	}

	wgr.Add(1)

	go func() {
		defer wgr.Done()

		err := server.Serve(listener)
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			errChan <- fmt.Errorf("can't start http server: %w", err)
		}
	}()

	wgr.Add(1)

	go func() {
		defer wgr.Done()

		<-ctx.Done()

		sdCtx := ctx
		if shutdownTimeout > 0 {
			var cancel context.CancelFunc
			sdCtx, cancel = context.WithTimeout(context.Background(), shutdownTimeout)
			defer cancel()
		}
		if err := server.Shutdown(sdCtx); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errChan <- fmt.Errorf("can't shutdown http server: %w", err)
		}
	}()

	return nil
}
