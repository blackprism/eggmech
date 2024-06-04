package core

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"time"

	"github.com/disgoorg/disgo/bot"
    "github.com/disgoorg/disgo/events"
	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
)

var ErrNatConnectionNil = errors.New("nat connection is nil")

const sleepBetweenConsumeFailure = 1 * time.Second

type Stream struct {
	Name string
	Register func(ctx context.Context, client bot.Client)
}

func GetStreams(nc *nats.Conn) ([]Stream, error) {
	if nc == nil {
		return []Stream{}, ErrNatConnectionNil
	}

	return []Stream{
		{
			Register: func(ctx context.Context, client bot.Client) {
				js, err := jetstreamConnect(
					ctx, 
					nc, 
					"MESSAGE", 
					[]string{
						"message.>",
					},
				)

				if err != nil {
					return
				}

				client.AddEventListeners(&events.ListenerAdapter{
					OnMessageCreate: MessageHandler(ctx, js, client.ID(), "message"),
				})
			},
		},
		{
			Register: func(ctx context.Context, client bot.Client) {
				js, err := jetstreamConnect(
					ctx, 
					nc, 
					"ACTIVITY", 
					[]string{
						"activity.>",
					},
				)

				if err != nil {
					return
				}

				client.AddEventListeners(&events.ListenerAdapter{
					OnUserActivityStart: ActivityStartHandler(ctx, js, client.ID(), "activity"),
				})
			},
		},
	}, nil


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

func jetstreamConnect(ctx context.Context, nc *nats.Conn, name string, subjects []string) (jetstream.JetStream, error) {
	js, err := jetstream.New(nc)

	if err != nil {
		slog.Error("Error creating jetstream instance", slog.Any("error", err))
		return nil, err
	}

	cfg := jetstream.StreamConfig{
		Name:       name,
		Subjects:   subjects,
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

func Consume(ctx context.Context, consumerName string, streamName string, subjects []string, handler func(msg jetstream.Msg) error) error {
	ctx, cancel := signal.NotifyContext(ctx, os.Interrupt)
	defer cancel()

	slog.Info(fmt.Sprintf("%s is now running. Press CTRL-C to exit.", consumerName))

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

	dur, err := durableConsumer(ctx, consumerName, stream, subjects)

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

func durableConsumer(ctx context.Context, consumerName string, stream jetstream.Stream, subjects []string) (jetstream.Consumer, error) {
	dur, err := stream.Consumer(ctx, consumerName)

	if err == nil {
		return dur, nil
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
		FilterSubjects: subjects,
	})

	if errCreateConsumer != nil {
		slog.Error("Error creating consumer", slog.String("consumer", consumerName), slog.Any("error", err))
		return nil, errCreateConsumer
	}

	return dur, nil
}
