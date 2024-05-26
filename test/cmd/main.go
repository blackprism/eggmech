package main

import (
	"context"
	"os"

	"eggmech/test/internal"
)

func main() {
	ctx := context.Background()

	os.Exit(internal.Run(ctx))
}
