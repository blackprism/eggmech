package main

import (
	"context"
	"os"

	"eggmech/autoChannelActivity/internal"
)

func main() {
	ctx := context.Background()

	err := internal.Run(ctx)

	if err != nil {
		os.Exit(1)
	}

	os.Exit(0)
}
