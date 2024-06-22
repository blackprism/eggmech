package autochannelactivity

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"strings"

	"github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/disgo/events"
	"github.com/disgoorg/disgo/rest"
	disgojson "github.com/disgoorg/json"
	"github.com/disgoorg/snowflake/v2"
	"github.com/gofrs/uuid/v5"
	"github.com/gosimple/slug"
	"github.com/nats-io/nats.go/jetstream"
	"github.com/samber/oops"

	"eggmech/core"
)

const Name = "autoChannelActivity"

func Run(ctx context.Context, getenv func(string) string) error {
	ctx, cancel := signal.NotifyContext(ctx, os.Interrupt)
	defer cancel()

	natsConn, err := core.Connect(getenv("NATS_URL"), -1)

	if err != nil {
		return oops.Wrapf(err, "failed to connect to nats")
	}

	db, err := sql.Open("sqlite3", "deployments/data/database.sqlite3")
	if err != nil {
		slog.Error("failed to connect to database", slog.Any("error", err))
	}
	defer db.Close()

	repo := Repository{DB: db}

	insertStatement, err := repo.InsertStatement(ctx)
	if err != nil {
		slog.Error("failed to get insert statement", slog.Any("error", err))
	}
	defer insertStatement.Close()

	closeActivityStatement, err := repo.CloseActivityStatement(ctx)
	if err != nil {
		slog.Error("failed to get close activity statement", slog.Any("error", err))
	}
	defer closeActivityStatement.Close()

	client := rest.New(rest.NewClient(getenv("DISCORD_TOKEN")))

	if err != nil {
		return oops.Wrapf(err, "error connecting to disgo")
	}

	handler := func(client rest.Rest, insertStatement *sql.Stmt, closeActivityStatement *sql.Stmt) func(msg jetstream.Msg) error {
		return func(msg jetstream.Msg) error {

			var event *events.PresenceUpdate
			err = json.Unmarshal(msg.Data(), &event)

			if err != nil {
				return oops.Wrapf(err, "failed to unmarshal presence update event")
			}

			slog.Info("received from durable consumer", slog.Any("event", event), slog.Any("subject", msg.Subject()))

			currentActivities, err := repo.GetCurrentActivitiesUUID(ctx, event)

			if err != nil {
				return oops.Wrapf(err, "failed to get current activities")
			}

			processActivitiesToClose(ctx, client, closeActivityStatement, event, currentActivities)
			processActivitiesToCreate(ctx, client, insertStatement, event, currentActivities)

			return nil
		}
	}

	return core.ConsumeActivity(
		ctx,
		natsConn,
		Name,
		[]string{"activity.gaming"},
		handler(client, insertStatement, closeActivityStatement),
	)
}

func processActivitiesToClose(
	ctx context.Context,
	client rest.Rest,
	closeActivityStatement *sql.Stmt,
	event *events.PresenceUpdate,
	currentActivities []CurrentActivity,
) {
	_ = client
	for _, currentActivity := range currentActivities {
		found := false

		for _, activity := range event.Activities {
			if activity.Name == currentActivity.Name {
				found = true

				break
			}
		}

		if !found {
			_, err := closeActivityStatement.ExecContext(ctx, currentActivity.UUID)
			if err != nil {
				slog.Error("failed to close activity", slog.Any("error", oops.Wrap(err)))
			}

			channels, err := client.GetGuildChannels(event.GuildID)
			if err != nil {
				slog.Error("failed to get channels", slog.Any("error", oops.Wrap(err)))
			}

			name := "te-" + slug.Make(currentActivity.Name)
			var channelFound snowflake.ID
			var channelFoundCategory snowflake.ID
			var categoryArchived snowflake.ID
			var categoryGame snowflake.ID

			for _, channel := range channels {
				if channel.Name() == name {
					channelFound = channel.ID()
					channelFoundCategory = *channel.ParentID()
				}

				if channel.Name() == "archive-game" && channel.Type() == discord.ChannelTypeGuildCategory {
					categoryArchived = channel.ID()
				}

				if channel.Type() == discord.ChannelTypeGuildCategory && strings.ToLower(channel.Name()) == "game" {
					categoryGame = channel.ID()
				}
			}

			if channelFound == 0 {
				continue
			}

			var channelsToUpdate []discord.GuildChannelPositionUpdate
			var channelPosition int

			for _, channel := range channels {
				if channel.ParentID() != nil && *channel.ParentID() == categoryArchived && channel.Name() < name {
					channelPosition = channel.Position() + 1
				}
			}

			channelsToUpdate = append(channelsToUpdate, discord.GuildChannelPositionUpdate{
				ID:       channelFound,
				ParentID: &categoryArchived,
				Position: disgojson.NewNullablePtr(channelPosition),
			})

			for _, channel := range channels {
				if channel.ParentID() != nil && *channel.ParentID() == categoryArchived && channelPosition > 0 && channel.Position() >= channelPosition {
					channelsToUpdate = append(channelsToUpdate, discord.GuildChannelPositionUpdate{
						ID:       channel.ID(),
						Position: disgojson.NewNullablePtr(channel.Position() + 1),
					})
				}
			}

			if categoryArchived == 0 {
				archiveChannel, err := client.CreateGuildChannel(event.GuildID, discord.GuildCategoryChannelCreate{
					Name:                 "archive-game",
					Position:             0,
					PermissionOverwrites: nil,
				})

				if err != nil {
					slog.Error("cannot create category game", slog.Any("error", oops.Wrap(err)))
					return
				}

				categoryArchived = archiveChannel.ID()
			}

			if categoryGame == 0 {
				gameChannel, err := client.CreateGuildChannel(event.GuildID, discord.GuildCategoryChannelCreate{
					Name:                 "game",
					Position:             0,
					PermissionOverwrites: nil,
				})

				if err != nil {
					slog.Error("cannot create category game", slog.Any("error", oops.Wrap(err)))
					return
				}

				categoryGame = gameChannel.ID()
			}

			slog.Info("channel to update", slog.Any("channel", channelFound))
			if channelFoundCategory == categoryGame {
				client.UpdateChannelPositions(event.GuildID, []discord.GuildChannelPositionUpdate{
					{
						ID:       channelFound,
						ParentID: &categoryArchived,
						Position: disgojson.NewNullablePtr(channelPosition),
					},
				})
			}
		}
	}
}

func processActivitiesToCreate(
	ctx context.Context,
	client rest.Rest,
	insertStatement *sql.Stmt,
	event *events.PresenceUpdate,
	currentActivities []CurrentActivity,
) {
	for _, eventActivity := range event.Activities {
		found := false

		for _, activity := range currentActivities {
			if activity.Name == eventActivity.Name {
				found = true

				break
			}
		}

		if !found {
			slog.Warn("1. NEWWWWWWWWW ACTIVITY")

			uuidv7, err := uuid.NewV7()
			if err != nil {
				slog.Error("failed to generate uuid", slog.Any("error", oops.Wrap(err)))
			}

			_, err = insertStatement.ExecContext(
				ctx,
				uuidv7,
				event.GuildID,
				event.PresenceUser.ID,
				eventActivity.Name,
				eventActivity.CreatedAt,
			)
			if err != nil {
				slog.Error("failed to insert activity", slog.Any("error", oops.Wrap(err)))
			}

			channels, err := client.GetGuildChannels(event.GuildID)
			if err != nil {
				slog.Error("failed to get channels", slog.Any("error", oops.Wrap(err)))
			}

			name := "te-" + slug.Make(eventActivity.Name)
			var channelFound snowflake.ID
			var channelFoundCategory snowflake.ID
			var categoryArchived snowflake.ID
			var categoryGame snowflake.ID

			for _, channel := range channels {
				if channel.Name() == name {
					channelFound = channel.ID()
					channelFoundCategory = *channel.ParentID()
				}

				if channel.Name() == "archive-game" && channel.Type() == discord.ChannelTypeGuildCategory {
					categoryArchived = channel.ID()
				}

				if channel.Type() == discord.ChannelTypeGuildCategory && strings.ToLower(channel.Name()) == "game" {
					categoryGame = channel.ID()
				}
			}

			var channelsToUpdate []discord.GuildChannelPositionUpdate
			var channelPosition int

			for _, channel := range channels {
				println(channel.Position(), channel.Name())
				if channel.ParentID() != nil && *channel.ParentID() == categoryGame && channel.Name() < name {
					channelPosition = channel.Position() + 1
					println("position", channelPosition)
				}
			}

			channelsToUpdate = append(channelsToUpdate, discord.GuildChannelPositionUpdate{
				ID:       channelFound,
				ParentID: &categoryGame,
				Position: disgojson.NewNullablePtr(channelPosition),
			})

			for _, channel := range channels {
				if channel.ParentID() != nil && *channel.ParentID() == categoryGame && channelPosition > 0 && channel.Position() >= channelPosition {
					channelsToUpdate = append(channelsToUpdate, discord.GuildChannelPositionUpdate{
						ID:       channel.ID(),
						Position: disgojson.NewNullablePtr(channel.Position() + 1),
					})
				}
			}

			if categoryGame == 0 {
				gameChannel, err := client.CreateGuildChannel(event.GuildID, discord.GuildCategoryChannelCreate{
					Name:                 "game",
					Position:             0,
					PermissionOverwrites: nil,
				})

				if err != nil {
					slog.Error("cannot create category game", slog.Any("error", oops.Wrap(err)))
					return
				}

				categoryGame = gameChannel.ID()
			}

			if categoryArchived == 0 {
				archiveChannel, err := client.CreateGuildChannel(event.GuildID, discord.GuildCategoryChannelCreate{
					Name:                 "archive-game",
					Position:             0,
					PermissionOverwrites: nil,
				})

				if err != nil {
					slog.Error("cannot create category game", slog.Any("error", oops.Wrap(err)))
					return
				}

				categoryArchived = archiveChannel.ID()
			}

			if channelFound == 0 {
				_, err = client.CreateGuildChannel(event.GuildID, discord.GuildTextChannelCreate{
					Name:     name,
					ParentID: categoryGame,
				})
				if err != nil {
					slog.Error("cannot create channel", slog.Any("error", oops.Wrap(err)))
					return
				}

				continue
			}

			if channelFoundCategory == categoryArchived {
				client.UpdateChannelPositions(event.GuildID, channelsToUpdate)
				for _, channelToUpdate := range channelsToUpdate {
					fmt.Printf("%d %v\n", channelToUpdate.ID, *channelToUpdate.Position)
				}
			}
		}
	}
}
