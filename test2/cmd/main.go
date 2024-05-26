package main

import (
	"context"
	"os"

	"eggmech/test2/internal"
)

func main() {
	ctx := context.Background()

	os.Exit(internal.Run(ctx))
}
