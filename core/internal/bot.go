package internal

import (
    "context"
    "log/slog"
    "os"
    "os/signal"

    "eggmech/core"

    "github.com/disgoorg/disgo"
    "github.com/disgoorg/disgo/bot"
    "github.com/disgoorg/disgo/gateway"
)

func Run(ctx context.Context, getenv func(string) string) int {
    ctx, cancel := signal.NotifyContext(ctx, os.Interrupt)
    defer cancel()

    nc, err := core.Connect()

    if err != nil {
        return 1
    }

    defer core.Close(nc)

    client, err := disgo.New(getenv("DISCORD_TOKEN"),
        bot.WithGatewayConfigOpts(
            gateway.WithIntents(
                gateway.IntentGuilds,
                gateway.IntentGuildMessages,
                gateway.IntentDirectMessages,
            ),
        ),
    )   

    if err != nil {
        slog.Error("Failed to create disgo", slog.Any("error", err))
        return 1
    }

    streams, err := core.GetStreams(nc)

    if err != nil {
        slog.Error("Failed to get streams", slog.Any("error", err))
        return 1
    }

    for _, stream := range streams {
        stream.Register(ctx, client)
    }                           

    if err = client.OpenGateway(ctx); err != nil {
        slog.Error("Failed to open gateway", slog.Any("error", err))
        return 1
    }

    slog.Info("Bot is now running. Press CTRL-C to exit.")
    <-ctx.Done()

    return 0
}
