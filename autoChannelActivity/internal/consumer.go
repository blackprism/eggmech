package internal

import (
	"context"
	"fmt"
	
	"eggmech/core"

	"github.com/nats-io/nats.go/jetstream"
)

func Run(ctx context.Context) error {
	return core.Consume(ctx, "autoChannelActivity", func(msg jetstream.Msg) error {
		fmt.Printf("received %q from durable consumer\n", msg.Data())
		return nil
	})
}
