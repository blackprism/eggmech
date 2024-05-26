package internal

import (
	"context"
	"errors"
	"fmt"
	"os"
	"time"
	
	"eggmech/core"

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

	stream, err := js.Stream(ctx, core.Streams[0].Name)
	if err != nil {
		println("3")
		fmt.Fprintln(os.Stderr, err)
		return 1
	}

	//stream.DeleteConsumer(ctx, "testConsumer2")
	//return 1

	dur, err := stream.Consumer(ctx, "testConsumer2")

	if errors.Is(err, jetstream.ErrConsumerNotFound) {
		msg, errLastMessage := stream.GetLastMsgForSubject(ctx, "message.srm")

		var startSeq uint64 = 0
		if errLastMessage != nil {
			println("3.1", errLastMessage.Error())
		}

		if errLastMessage == nil {
			startSeq = msg.Sequence + 1
		}

		dur, errLastMessage = stream.CreateConsumer(ctx, jetstream.ConsumerConfig{
			Durable:       "testConsumer2",
			DeliverPolicy: jetstream.DeliverByStartSequencePolicy,
			OptStartSeq:   startSeq,
		})
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
