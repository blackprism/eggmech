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

func Consume(ctx context.Context, name string, handler func(msg jetstream.Msg) bool) int {
    ctx, cancel := signal.NotifyContext(ctx, os.Interrupt)
    defer cancel()

    slog.Info(fmt.Sprintf("%s is now running. Press CTRL-C to exit.", name))

    nc, err := nats.Connect(nats.DefaultURL)

    if err != nil {
        slog.Error("Error connecting to nats", slog.Any("error", err))
        return 1
    }

    defer func(nc *nats.Conn) {
        err := nc.Drain()
        if err != nil {
            slog.Error("Error draining nats", slog.Any("error", err))
        }
    }(nc)

    js, err := jetstream.New(nc)
    if err != nil {
        slog.Error("Error creating jetstream", slog.Any("error", err))
        return 1
    }

    stream, err := js.Stream(ctx, GetStreams()[0].Name)
    if err != nil {
        slog.Error("Error creating stream", slog.String("name", GetStreams()[0].Name), slog.Any("error", err))
        return 1
    }

    //stream.DeleteConsumer(ctx, "testConsumer2")
    //return 1

    dur, err := stream.Consumer(ctx, name)

    if errors.Is(err, jetstream.ErrConsumerNotFound) {
        msg, errLastMessage := stream.GetLastMsgForSubject(ctx, "message.srm")

        var startSeq uint64 = 1
        if errLastMessage != nil {
            println("3.1", errLastMessage.Error())
        }

        if errLastMessage == nil {
            startSeq = msg.Sequence + 1
        }

        println(startSeq)

        dur, errLastMessage = stream.CreateConsumer(ctx, jetstream.ConsumerConfig{
            Durable:       "testConsumer2",
            DeliverPolicy: jetstream.DeliverByStartSequencePolicy,
            OptStartSeq:   startSeq,
        })

        if errLastMessage != nil {
            println("4", errLastMessage.Error())
        }
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
