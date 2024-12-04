package autochannelactivity

import (
	"context"
	"database/sql"
	"embed"
	"log/slog"
	"os"
	"os/signal"

	"github.com/disgoorg/disgo"
	"github.com/disgoorg/disgo/bot"
	"github.com/disgoorg/disgo/events"
	"github.com/disgoorg/disgo/gateway"
	"github.com/disgoorg/disgo/rest"
	"github.com/samber/oops"
)

const Name = "autoChannelActivity"
const categoryGame = "game"
const categoryArchive = "game archive"

//go:embed migrations/*.sql
var migrationsEmbed embed.FS

func Run(ctx context.Context, getenv func(string) string) error {
	ctx, cancel := signal.NotifyContext(ctx, os.Interrupt)
	defer cancel()

	err := migration(migrationsEmbed)

	if err != nil {
		return oops.Wrapf(err, "failed to run migration")
	}

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

	db, err := sql.Open("sqlite3", "deployments/data/database.sqlite3")
	if err != nil {
		slog.Error("failed to connect to database", slog.Any("error", err))

		return err
	}
	defer db.Close()

	repo := Repository{DB: db}
	client := rest.New(rest.NewClient(getenv("DISCORD_TOKEN")))

	discord.AddEventListeners(&events.ListenerAdapter{
		OnPresenceUpdate: PresenceHandler(ctx, discord.ID(), client, repo),
	})

	if err = discord.OpenGateway(ctx); err != nil {
		return oops.Wrapf(err, "error connecting to Discord")
	}

	slog.Info("Bot module autochannelactivity is now running. Press CTRL-C to exit.")
	<-ctx.Done()

	return nil
}
