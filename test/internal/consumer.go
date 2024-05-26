package internal

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
)

func Run(ctx context.Context) int {
	nc, err := nats.Connect(nats.DefaultURL)

	if err != nil {
		println("1")
		fmt.Fprintln(os.Stderr, err)
		return 1
	}

	defer nc.Drain()

	js, err := jetstream.New(nc)
	if err != nil {
		println("2")
		fmt.Fprintln(os.Stderr, err)
		return 1
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	cfg := jetstream.StreamConfig{
		Name:     "MESSAGE",
		Subjects: []string{"message.>"},
	}
	_ = cfg

	stream, err := js.Stream(ctx, "MESSAGE")
	if err != nil {
		println("3")
		fmt.Fprintln(os.Stderr, err)
		return 1
	}

	dur, err := stream.CreateOrUpdateConsumer(ctx, jetstream.ConsumerConfig{
		Durable: "testConsumer",
		//AckPolicy: jetstream.AckNonePolicy,
	})

	if err != nil {
		println("4")
		fmt.Fprintln(os.Stderr, err)
		return 1
	}

	for {
		msgs, err := dur.Fetch(1)

		if err != nil {
			println("5")
			fmt.Fprintln(os.Stderr, err)
			return 1
		}

		msg := <-msgs.Messages()
		fmt.Printf("%+v\n", msg)

		if msg != nil {
			msg.Ack()

			fmt.Printf("received %q from durable consumer\n", msg.Data())
		}
	}

	return 0
}
