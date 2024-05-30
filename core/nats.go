package core

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
)

const sleepBetweenConsumeFailure = 1 * time.Second

type Stream struct {
	Name     string
	Subjects []string
}

func GetStreams() [1]Stream {
	return [1]Stream{
		{
			Name: "MESSAGE",
			Subjects: []string{
				"message.>",
			},
		},
	}
}

func Connect() (*nats.Conn, error) {
	nc, err := nats.Connect(nats.DefaultURL)

	if err != nil {
		slog.Error("Error connecting to nats", slog.Any("error", err))
		return nil, err
	}

	return nc, err
}

func Close(nc *nats.Conn) {
	if nc == nil {
		return
	}

	err := nc.Drain()
	if err != nil {
		slog.Error("Error draining nats", slog.Any("error", err))
	}

	nc.Close()
}

func Jetstream(ctx context.Context, nc *nats.Conn) (jetstream.JetStream, error) {
	js, err := jetstream.New(nc)

	if err != nil {
		slog.Error("Error creating jetstream instance", slog.Any("error", err))
		return nil, err
	}

	cfg := jetstream.StreamConfig{
		Name:       GetStreams()[0].Name,
		Subjects:   GetStreams()[0].Subjects,
		MaxAge:     7 * 24 * time.Hour,
		Duplicates: 10 * time.Second,
		Storage:    jetstream.FileStorage,
	}

	_, err = js.CreateOrUpdateStream(ctx, cfg)
	if err != nil {
		slog.Error("Error creating stream", slog.Any("error", err))
		return nil, err
	}

	return js, nil
}

func Consume(ctx context.Context, consumerName string, handler func(msg jetstream.Msg) error) error {
	ctx, cancel := signal.NotifyContext(ctx, os.Interrupt)
	defer cancel()

	slog.Info(fmt.Sprintf("%s is now running. Press CTRL-C to exit.", consumerName))
	streamName := GetStreams()[0].Name

	nc, err := Connect()

	if err != nil {
		return err
	}

	defer Close(nc)

	js, err := jetstream.New(nc)
	if err != nil {
		slog.Error("Error creating jetstream", slog.Any("error", err))
		return err
	}

	stream, err := js.Stream(ctx, streamName)
	if err != nil {
		slog.Error("Error creating stream", slog.String("consumer", consumerName), slog.String("stream", streamName), slog.Any("error", err))
		return err
	}

	//stream.DeleteConsumer(ctx, "testConsumer2")
	//return 1

	dur, err := durableConsumer(ctx, consumerName, stream)

	if err != nil {
		return err
	}

	for {
		msgs, errFetch := dur.Fetch(1)

		if errFetch != nil {
			slog.Error("Error fetching", slog.String("consumer", consumerName), slog.Any("error", errFetch))
			time.Sleep(sleepBetweenConsumeFailure)
			continue
		}

		select {
		case <-ctx.Done():
			return nil
		case msg := <-msgs.Messages():
			if msg != nil {
				errHandler := handler(msg)

				if errHandler != nil {
					slog.Error("Error consumer handler", slog.String("consumer", consumerName), slog.Any("error", errHandler))
					time.Sleep(sleepBetweenConsumeFailure)

					continue
				}

				if errHandler == nil {
					errAck := msg.Ack()
					if errAck != nil {
						slog.Error("Error consumer ack", slog.String("consumer", consumerName), slog.Any("error", errAck))
					}
				}
			}
		}
	}
}

func durableConsumer(ctx context.Context, consumerName string, stream jetstream.Stream) (jetstream.Consumer, error) {
	dur, err := stream.Consumer(ctx, consumerName)

	if err == nil {
		return dur, err
	}

	if !errors.Is(err, jetstream.ErrConsumerNotFound) {
		slog.Error("Error creating consumer", slog.String("consumer", consumerName), slog.Any("error", err))
		return nil, err
	}

	msg, errLastMessage := stream.GetLastMsgForSubject(ctx, "")

	var startSeq uint64 = 1

	if errLastMessage == nil {
		startSeq = msg.Sequence + 1
	}

	slog.Info(fmt.Sprintf("Sequence for consumer starts at %d", startSeq), slog.String("consumer", consumerName))

	dur, errCreateConsumer := stream.CreateConsumer(ctx, jetstream.ConsumerConfig{
		Durable:       consumerName,
		DeliverPolicy: jetstream.DeliverByStartSequencePolicy,
		OptStartSeq:   startSeq,
	})

	if errCreateConsumer != nil {
		slog.Error("Error creating consumer", slog.String("consumer", consumerName), slog.Any("error", err))
		return nil, errCreateConsumer
	}

	return dur, nil
}
