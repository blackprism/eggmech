package internal

import (
    "context"
    "log/slog"
    "os"
    "os/signal"
    "time"

    "eggmech/core"

    "github.com/disgoorg/disgo"
    "github.com/disgoorg/disgo/bot"
    "github.com/disgoorg/disgo/gateway"
    "github.com/nats-io/nats.go"
    "github.com/nats-io/nats.go/jetstream"
)

func Run(ctx context.Context, getenv func(string) string) int {
    ctx, cancel := signal.NotifyContext(ctx, os.Interrupt)
    defer cancel()

    nc, err := getNatsConnection()

    if err != nil {
        return 1
    }

    defer deferNc(nc)

    js, err := getJetstream(ctx, nc)

    if err != nil {
        return 1
    }

    client, err := disgo.New(getenv("DISCORD_TOKEN"),
        bot.WithGatewayConfigOpts(
            gateway.WithIntents(
                gateway.IntentGuilds,
                gateway.IntentGuildMessages,
                gateway.IntentDirectMessages,
            ),
        ),
        bot.WithEventListenerFunc(MessageHandler(ctx, js)),
    )
    if err != nil {
        slog.Error("Failed to create disgo", slog.Any("error", err))

        return 1
    }
    // connect to the gateway
    if err = client.OpenGateway(ctx); err != nil {
        slog.Error("Failed to open gateway", slog.Any("error", err))

        return 1
    }

    slog.Info("Bot is now running. Press CTRL-C to exit.")
    <-ctx.Done()

    return 0
}

func getNatsConnection() (*nats.Conn, error) {
    nc, err := nats.Connect(nats.DefaultURL)

    if err != nil {
        slog.Error("Error connecting to nats", slog.Any("error", err))
        return nil, err
    }

    return nc, nil
}

func deferNc(nc *nats.Conn) {
    err := nc.Drain()
    if err != nil {
        slog.Error("Error draining nats", slog.Any("error", err))
    }
}

func getJetstream(ctx context.Context, nc *nats.Conn) (jetstream.JetStream, error) {
    js, err := jetstream.New(nc)

    if err != nil {
        slog.Error("Error creating jetstream instance", slog.Any("error", err))
        return nil, err
    }

    cfg := jetstream.StreamConfig{
        Name:       core.GetStreams()[0].Name,
        Subjects:   core.GetStreams()[0].Subjects,
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
