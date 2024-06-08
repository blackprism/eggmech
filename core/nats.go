package core

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
	"github.com/samber/oops"
)

const sleepBetweenConsumeFailure = 1 * time.Second
const maxReconnects = -1

var StreamConfigMaxAge = 7 * 24 * time.Hour
var StreamConfigDuplicates = 10 * time.Second
var StreamConfigStorage = jetstream.FileStorage

func Connect(url string) (*nats.Conn, error) {
	nc, err := nats.Connect(url, nats.MaxReconnects(maxReconnects))

	if err != nil {
		return nil, oops.Wrapf(err, "error connecting to nats")
	}

	return nc, nil
}

func Close(nc *nats.Conn) {
	if nc == nil {
		return
	}

	err := nc.Drain()
	if err != nil {
		slog.Error("failed to drain nats connection", slog.Any("error", oops.Wrap(err)))
	}

	nc.Close()

	return
}

func JetstreamConnect(ctx context.Context, nc *nats.Conn, name string, subjects []string) (jetstream.JetStream, error) {
	js, err := jetstream.New(nc)

	if err != nil {
		return nil, oops.Wrapf(err, "error creating jetstream instance")
	}

	cfg := jetstream.StreamConfig{
		Name:       name,
		Subjects:   subjects,
		MaxAge:     StreamConfigMaxAge,
		Duplicates: StreamConfigDuplicates,
		Storage:    StreamConfigStorage,
	}

	err = createOrUpdateStream(ctx, js, cfg)

	if err != nil {
		return nil, oops.Wrapf(err, "error creating or updating stream")
	}

	return js, nil
}

func streamExists(ctx context.Context, js jetstream.JetStream, cfg jetstream.StreamConfig) bool {
	for streamName := range js.StreamNames(ctx).Name() {
		if streamName == cfg.Name {
			return true
		}
	}

	return false
}

func createOrUpdateStream(ctx context.Context, js jetstream.JetStream, cfg jetstream.StreamConfig) error {
	if streamExists(ctx, js, cfg) {
		_, err := js.UpdateStream(ctx, cfg)

		return oops.Wrapf(err, "error updating stream")
	}

	_, err := js.CreateStream(ctx, cfg)

	return oops.Wrapf(err, "error creating stream")
}

func ConsumeActivity(ctx context.Context, nc *nats.Conn, consumerName string, subjects []string, handler func(msg jetstream.Msg) error) error {
	return consume(ctx, nc, consumerName, "ACTIVITY", subjects, handler)
}

func consume(ctx context.Context, nc *nats.Conn, consumerName string, streamName string, subjects []string, handler func(msg jetstream.Msg) error) error {
	slog.Info(fmt.Sprintf("%s is now running. Press CTRL-C to exit.", consumerName))
	defer Close(nc)

	js, err := jetstream.New(nc)
	if err != nil {
		return oops.Wrapf(err, "error creating jetstream instance")
	}

	stream, err := js.Stream(ctx, streamName)
	if err != nil {
		return oops.
			With("consumer", consumerName).
			With("stream", streamName).
			Wrapf(err, "error creating stream")
	}

	dur, err := durableConsumer(ctx, consumerName, stream, subjects)

	if err != nil {
		return oops.Wrapf(err, "error creating durable consumer")
	}

	for {
		msgs, errFetch := dur.Fetch(1)

		if errFetch != nil {
			slog.Error("error fetching", slog.String("consumer", consumerName), slog.Any("error", oops.Wrap(errFetch)))
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
					slog.Error("error consumer handler", slog.String("consumer", consumerName), slog.Any("error", errHandler))
					time.Sleep(sleepBetweenConsumeFailure)

					continue
				}

				errAck := msg.Ack()
				if errAck != nil {
					slog.Error("error consumer ack", slog.String("consumer", consumerName), slog.Any("error", errAck))
				}
			}
		}
	}
}

func durableConsumer(ctx context.Context, consumerName string, stream jetstream.Stream, subjects []string) (jetstream.Consumer, error) {
	dur, errCreateConsumer := stream.CreateOrUpdateConsumer(ctx, jetstream.ConsumerConfig{
		Durable:        consumerName,
		DeliverPolicy:  jetstream.DeliverNewPolicy,
		FilterSubjects: subjects,
	})

	if errCreateConsumer != nil {
		return nil, oops.
			With("consumer", consumerName).
			Wrapf(errCreateConsumer, "error creating consumer")
	}

	return dur, nil
}
