package main

import (
	"context"
	"log/slog"
	"os"

	"eggmech/autochannelactivity/internal"
	"eggmech/core"
)

func main() {
	ctx := context.Background()

	core.SetupLogger()

	err := internal.Run(ctx, os.Getenv)

	if err != nil {
		slog.Error("failed to start server", slog.Any("error", err))
		os.Exit(1)
	}

	os.Exit(0)
}
