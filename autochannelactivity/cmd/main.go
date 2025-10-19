package main

import (
	"context"
	"log/slog"
	"os"

	"github.com/google/gops/agent"
	_ "github.com/joho/godotenv/autoload"

	"eggmech/autochannelactivity"
)

func main() {
	ctx := context.Background()

	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		AddSource: true,
	}))

	if err := agent.Listen(agent.Options{}); err != nil {
		logger.ErrorContext(ctx, "error creating gops agent", slog.Any("error", err))
	}

	err := autochannelactivity.Run(ctx, os.Getenv, logger)

	if err != nil {
		logger.ErrorContext(ctx, "failed to start server", slog.Any("error", err))
		os.Exit(1)
	}
}
