package core

import (
    "context"
    "errors"
    "fmt"
    "log/slog"
    "os"
    "os/signal"

    "github.com/nats-io/nats.go"
    "github.com/nats-io/nats.go/jetstream"
)

type Stream struct {
    Name     string
    Subjects []string
}

func GetStreams() []Stream {
    return []Stream{
        {
            Name: "MESSAGE",
            Subjects: []string{
                "message.>",
            },
        },
    }
}

func Connect() (*nats.Conn, bool) {
    nc, err := nats.Connect(nats.DefaultURL)

    if err != nil {
        slog.Error("Error connecting to nats", slog.Any("error", err))
        return nil, false
    }

    return nc, true
}

func Close(nc *nats.Conn) {
    err := nc.Drain()
    if err != nil {
        slog.Error("Error draining nats", slog.Any("error", err))
    }

    nc.Close()
}

func Consume(ctx context.Context, consumerName string, handler func(msg jetstream.Msg) bool) int {
    ctx, cancel := signal.NotifyContext(ctx, os.Interrupt)
    defer cancel()

    slog.Info(fmt.Sprintf("%s is now running. Press CTRL-C to exit.", consumerName))
    streamName := GetStreams()[0].Name

    nc, success := Connect()

    if !success {
        return 1
    }

    defer Close(nc)

    js, err := jetstream.New(nc)
    if err != nil {
        slog.Error("Error creating jetstream", slog.Any("error", err))
        return 1
    }

    stream, err := js.Stream(ctx, streamName)
    if err != nil {
        slog.Error("Error creating stream", slog.String("consumer", consumerName), slog.String("stream", streamName), slog.Any("error", err))
        return 1
    }

    //stream.DeleteConsumer(ctx, "testConsumer2")
    //return 1

    dur, success := durableConsumer(ctx, consumerName, stream)

    if !success {
        return 1
    }

    for {
        msgs, err := dur.Fetch(1)

        if err != nil {
            println("5")
            fmt.Fprintln(os.Stderr, err)
            return 1
        }

        select {
        case <-ctx.Done():
            return 0
        case msg := <-msgs.Messages():
            if msg != nil {
                if handler(msg) {
                    msg.Ack()
                }
            }
        }
    }
}

func durableConsumer(ctx context.Context, name string, stream jetstream.Stream) (jetstream.Consumer, bool) {
    dur, err := stream.Consumer(ctx, name)

    // il va me dire early return ou pas ?

    if errors.Is(err, jetstream.ErrConsumerNotFound) {
        msg, errLastMessage := stream.GetLastMsgForSubject(ctx, "")

        var startSeq uint64 = 1

        if errLastMessage == nil {
            startSeq = msg.Sequence + 1
        }

        slog.Info(fmt.Sprintf("Sequence for consumer starts at %d", startSeq), slog.String("name", name))

        dur, errCreateConsumer := stream.CreateConsumer(ctx, jetstream.ConsumerConfig{
            Durable:       name,
            DeliverPolicy: jetstream.DeliverByStartSequencePolicy,
            OptStartSeq:   startSeq,
        })

        if errCreateConsumer != nil {
            slog.Error("Error creating consumer", slog.String("consumer", name), slog.Any("error", err))
            return nil, false
        }
    }
    return dur, true
}
