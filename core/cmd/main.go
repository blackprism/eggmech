package main

import (
	"context"
	"log/slog"
	"os"

	"github.com/google/gops/agent"
	_ "github.com/joho/godotenv/autoload"

	"eggmech/core"
	"eggmech/core/internal"
)

func main() {
	ctx := context.Background()

	if err := agent.Listen(agent.Options{}); err != nil {
		slog.Error("error creating agent", slog.Any("error", err))
	}

	core.SetupLogger()

	err := internal.Run(ctx, os.Getenv)

	if err != nil {
		slog.Error("failed to start server", slog.Any("error", err))
		os.Exit(1)
	}
}
