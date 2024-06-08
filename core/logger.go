package core

import (
	"log/slog"
	"os"
)

func SetupLogger() {
	logger := slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		AddSource: true,
	})

	slog.SetDefault(slog.New(logger))
}
