package internal

import (
	"context"
	"log/slog"
	"os"
	"os/signal"

	"github.com/disgoorg/disgo"
	"github.com/disgoorg/disgo/bot"
	"github.com/disgoorg/disgo/events"
	"github.com/disgoorg/disgo/gateway"
	"github.com/samber/oops"

	"eggmech/core"
)

func Run(ctx context.Context, getenv func(string) string) error {
	ctx, cancel := signal.NotifyContext(ctx, os.Interrupt)
	defer cancel()

	nc, err := core.Connect(getenv("NATS_URL"))

	if err != nil {
		return oops.Wrapf(err, "error connecting to nats server")
	}

	defer core.Close(nc)

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

	js, err := core.JetstreamConnect(
		ctx,
		nc,
		"ACTIVITY",
		[]string{
			"activity.>",
		},
	)

	if err != nil {
		return oops.Wrapf(err, "error connecting to Jetstream")
	}

	discord.AddEventListeners(&events.ListenerAdapter{
		OnPresenceUpdate: core.PresenceHandler(ctx, js, discord.ID(), "activity"),
	})

	if err = discord.OpenGateway(ctx); err != nil {
		return oops.Wrapf(err, "error connecting to Discord")
	}

	slog.Info("Bot is now running. Press CTRL-C to exit.")
	<-ctx.Done()

	return nil
}
