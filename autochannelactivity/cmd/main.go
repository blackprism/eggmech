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

	if err := agent.Listen(agent.Options{}); err != nil {
		slog.Error("error creating gops agent", slog.Any("error", err))
	}

	autochannelactivity.SetupLogger()

	err := autochannelactivity.Run(ctx, os.Getenv)

	if err != nil {
		slog.Error("failed to start server", slog.Any("error", err))
		os.Exit(1)
	}
}
