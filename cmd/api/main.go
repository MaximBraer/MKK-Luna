package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"MKK-Luna/internal/application"
)

const build = "dev"

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	app := application.New()
	if err := app.Start(ctx, build); err != nil {
		stop()
		os.Exit(1)
	}

	if err := app.Wait(ctx, stop); err != nil {
		os.Exit(1)
	}
}
