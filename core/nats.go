package core

import (
	"context"
	"log/slog"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
	"github.com/samber/oops"
)

func Connect(url string, maxReconnects int) (*nats.Conn, error) {
	natsConn, err := nats.Connect(url, nats.MaxReconnects(maxReconnects))

	if err != nil {
		return nil, oops.Wrapf(err, "error connecting to nats")
	}

	return natsConn, nil
}

func Close(natsConn *nats.Conn) {
	if natsConn == nil {
		return
	}

	err := natsConn.Drain()
	if err != nil {
		slog.Error("failed to drain nats connection", slog.Any("error", oops.Wrap(err)))
	}

	natsConn.Close()
}

func JetstreamConnect(
	ctx context.Context,
	natsConn *nats.Conn,
	name string,
	subjects []string,
	maxAge time.Duration,
	duplicates time.Duration,
	storage jetstream.StorageType,
) (jetstream.JetStream, error) {
	jsConn, err := jetstream.New(natsConn)

	if err != nil {
		return nil, oops.Wrapf(err, "error creating jetstream instance")
	}

	cfg := jetstream.StreamConfig{
		Name:       name,
		Subjects:   subjects,
		MaxAge:     maxAge,
		Duplicates: duplicates,
		Storage:    storage,
	}

	err = createOrUpdateStream(ctx, jsConn, cfg)

	if err != nil {
		return nil, oops.Wrapf(err, "error creating or updating stream")
	}

	return jsConn, nil
}

func streamExists(ctx context.Context, js jetstream.JetStream, cfg jetstream.StreamConfig) bool {
	for streamName := range js.StreamNames(ctx).Name() {
		if streamName == cfg.Name {
			return true
		}
	}

	return false
}

func createOrUpdateStream(ctx context.Context, jsConn jetstream.JetStream, cfg jetstream.StreamConfig) error {
	if streamExists(ctx, jsConn, cfg) {
		_, err := jsConn.UpdateStream(ctx, cfg)

		return oops.Wrapf(err, "error updating stream")
	}

	_, err := jsConn.CreateStream(ctx, cfg)

	return oops.Wrapf(err, "error creating stream")
}

func ConsumeActivity(
	ctx context.Context,
	natsConn *nats.Conn,
	consumerName string,
	subjects []string,
	handler func(msg jetstream.Msg) error,
) error {
	streamName := "ACTIVITY"

	slog.Info("%s is now running. Press CTRL-C to exit.", consumerName)

	defer Close(natsConn)

	sleepBetweenConsumeFailure := 2 * time.Second

	jsConn, err := jetstream.New(natsConn)
	if err != nil {
		return oops.Wrapf(err, "error creating jetstream instance")
	}

	stream, err := jsConn.Stream(ctx, streamName)
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
			handleMessage(consumerName, handler, sleepBetweenConsumeFailure, msg)
		}
	}
}

func handleMessage(
	consumerName string,
	handler func(msg jetstream.Msg) error,
	sleepBetweenConsumeFailure time.Duration,
	msg jetstream.Msg,
) {
	if msg == nil {
		return
	}

	errHandler := handler(msg)

	if errHandler != nil {
		slog.Error("error consumer handler", slog.String("consumer", consumerName), slog.Any("error", errHandler))
		time.Sleep(sleepBetweenConsumeFailure)

		return
	}

	errAck := msg.Ack()
	if errAck != nil {
		slog.Error("error consumer ack", slog.String("consumer", consumerName), slog.Any("error", errAck))
	}
}

func durableConsumer(
	ctx context.Context,
	consumerName string,
	stream jetstream.Stream,
	subjects []string,
) (jetstream.Consumer, error) {
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
