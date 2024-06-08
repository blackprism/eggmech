package internal

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"time"

	"github.com/disgoorg/disgo"
	"github.com/disgoorg/disgo/bot"
	"github.com/disgoorg/disgo/events"
	"github.com/disgoorg/disgo/gateway"
	"github.com/nats-io/nats.go/jetstream"
	"github.com/samber/oops"

	"eggmech/core"
)

func Run(ctx context.Context, getenv func(string) string) error {
	ctx, cancel := signal.NotifyContext(ctx, os.Interrupt)
	defer cancel()

	natsConn, err := core.Connect(getenv("NATS_URL"), -1)

	if err != nil {
		return oops.Wrapf(err, "error connecting to nats server")
	}

	defer core.Close(natsConn)

	discord, err := disgo.New(getenv("DISCORD_TOKEN"),
		bot.WithGatewayConfigOpts(
			gateway.WithIntents(
				gateway.IntentGuilds,
				gateway.IntentGuildMessages,
				gateway.IntentDirectMessages,
				gateway.IntentGuildPresences,
			),
		),
	)

	if err != nil {
		return oops.Wrapf(err, "error connecting to disgo")
	}

	jsConn, err := core.JetstreamConnect(
		ctx,
		natsConn,
		"ACTIVITY",
		[]string{
			"activity.>",
		},
		7*24*time.Hour,
		10*time.Second,
		jetstream.FileStorage,
	)

	if err != nil {
		return oops.Wrapf(err, "error connecting to Jetstream")
	}

	discord.AddEventListeners(&events.ListenerAdapter{
		OnPresenceUpdate: core.PresenceHandler(ctx, jsConn, discord.ID(), "activity"),
	})

	if err = discord.OpenGateway(ctx); err != nil {
		return oops.Wrapf(err, "error connecting to Discord")
	}

	slog.Info("Bot is now running. Press CTRL-C to exit.")
	<-ctx.Done()

	return nil
}
