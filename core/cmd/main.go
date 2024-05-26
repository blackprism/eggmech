package main

import (
	"context"
	"log/slog"
	"os"

	"eggmech/core/internal"

	"github.com/google/gops/agent"
	_ "github.com/joho/godotenv/autoload"
)

func main() {
	ctx := context.Background()

	if err := agent.Listen(agent.Options{}); err != nil {
		slog.Error("Error creating agent", slog.Any("error", err))
	}

	os.Exit(internal.Run(ctx, os.Getenv))
}
