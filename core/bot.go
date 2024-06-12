package core

import (
	"context"
	"embed"
	"log/slog"
	"net/url"
	"os"
	"os/signal"
	"time"

	"github.com/amacneil/dbmate/v2/pkg/dbmate"
	_ "github.com/amacneil/dbmate/v2/pkg/driver/sqlite"
	"github.com/disgoorg/disgo"
	"github.com/disgoorg/disgo/bot"
	"github.com/disgoorg/disgo/events"
	"github.com/disgoorg/disgo/gateway"
	"github.com/nats-io/nats.go/jetstream"
	"github.com/samber/oops"
)

//go:embed migrations/*.sql
var migrationsEmbed embed.FS

func migration() error {
	databaseURL, _ := url.Parse("sqlite:deployments/data/database.sqlite3")
	db := dbmate.New(databaseURL)
	db.MigrationsDir = []string{"migrations"}
	db.FS = migrationsEmbed
	db.SchemaFile = "deployments/data/database-schema.sql"

	migrations, err := db.FindMigrations()
	if err != nil {
		return oops.Wrapf(err, "failed to find migrations")
	}

	for _, m := range migrations {
		slog.Info("Migration", slog.String("version", m.Version), slog.String("file", m.FilePath))
	}

	err = db.CreateAndMigrate()
	if err != nil {
		return oops.Wrapf(err, "failed to create migration")
	}

	return nil
}

func Run(ctx context.Context, getenv func(string) string) error {
	ctx, cancel := signal.NotifyContext(ctx, os.Interrupt)
	defer cancel()

	err := migration()

	if err != nil {
		return oops.Wrapf(err, "failed to run migration")
	}

	natsConn, err := Connect(getenv("NATS_URL"), -1)

	if err != nil {
		return oops.Wrapf(err, "error connecting to nats server")
	}

	defer Close(natsConn)

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

	jsConn, err := JetstreamConnect(
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
		OnPresenceUpdate: PresenceHandler(ctx, jsConn, discord.ID(), "activity"),
	})

	if err = discord.OpenGateway(ctx); err != nil {
		return oops.Wrapf(err, "error connecting to Discord")
	}

	slog.Info("Bot is now running. Press CTRL-C to exit.")
	<-ctx.Done()

	return nil
}
