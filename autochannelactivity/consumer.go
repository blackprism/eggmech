package autochannelactivity

import (
	"context"
	"database/sql"
	"encoding/json"
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
const categoryGame = "game"
const categoryArchive = "game archive"

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

	client := rest.New(rest.NewClient(getenv("DISCORD_TOKEN")))

	if err != nil {
		return oops.Wrapf(err, "error connecting to disgo")
	}

	return core.ConsumeActivity(
		ctx,
		natsConn,
		Name,
		[]string{"activity.gaming"},
		handler(ctx, db, client),
	)
}

func handler(ctx context.Context, db *sql.DB, client rest.Rest) func(msg jetstream.Msg) error {
	repo := Repository{DB: db}

	insertStatement, err := repo.InsertStatement(ctx)
	if err != nil {
		slog.Error("failed to get insert statement", slog.Any("error", err))
	}

	closeActivityStatement, err := repo.CloseActivityStatement(ctx)
	if err != nil {
		slog.Error("failed to get close activity statement", slog.Any("error", err))
	}

	return func(msg jetstream.Msg) error {
		var event *events.PresenceUpdate
		errUnmarshal := json.Unmarshal(msg.Data(), &event)

		if errUnmarshal != nil {
			return oops.Wrapf(errUnmarshal, "failed to unmarshal presence update event")
		}

		slog.Info("received from durable consumer", slog.Any("event", event), slog.Any("subject", msg.Subject()))

		currentActivities, errRetrievingActivities := repo.GetCurrentActivitiesUUID(ctx, event)

		if errRetrievingActivities != nil {
			return oops.Wrapf(errRetrievingActivities, "failed to get current activities")
		}

		processActivitiesToClose(ctx, client, closeActivityStatement, event, currentActivities, repo)
		processActivitiesToCreate(ctx, client, insertStatement, event, currentActivities, repo)

		return nil
	}
}

func processActivitiesToClose(
	ctx context.Context,
	client rest.Rest,
	closeActivityStatement *sql.Stmt,
	event *events.PresenceUpdate,
	currentActivities []CurrentActivity,
	repo Repository,
) {
	for _, currentActivity := range currentActivities {
		foundInActivity := false

		for _, activity := range event.Activities {
			if activity.Name == currentActivity.Name {
				foundInActivity = true

				break
			}
		}

		if !foundInActivity {
			_, err := closeActivityStatement.ExecContext(ctx, currentActivity.UUID)
			if err != nil {
				slog.Error("failed to close activity", slog.Any("error", oops.Wrap(err)))
			}

			processActivity(
				ctx,
				client,
				event,
				currentActivity,
				repo,
			)
		}
	}
}

func processActivitiesToCreate(
	ctx context.Context,
	client rest.Rest,
	insertStatement *sql.Stmt,
	event *events.PresenceUpdate,
	currentActivities []CurrentActivity,
	repo Repository,
) {
	for _, eventActivity := range event.Activities {
		foundInDatabase := false

		for _, activity := range currentActivities {
			if activity.Name == eventActivity.Name {
				foundInDatabase = true

				break
			}
		}

		if !foundInDatabase {
			uuidv7, err := uuid.NewV7()
			if err != nil {
				slog.Error("failed to generate uuid", slog.Any("error", oops.Wrap(err)))

				return
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

				return
			}

			processActivity(
				ctx,
				client,
				event,
				CurrentActivity{
					UUID: "",
					Name: eventActivity.Name,
				},
				repo,
			)
		}
	}
}

func processActivity(
	ctx context.Context,
	client rest.Rest,
	event *events.PresenceUpdate,
	activity CurrentActivity,
	repo Repository,
) {
	hasEnoughActivityUsage, err := repo.HasEnoughActivityUsage(ctx, activity.Name)
	if err != nil {
		slog.Error("failed to get game usage", slog.Any("error", oops.Wrap(err)))

		return
	}

	channels, err := client.GetGuildChannels(event.GuildID)
	if err != nil {
		slog.Error("failed to get channels", slog.Any("error", oops.Wrap(err)))

		return
	}

	name := slug.Make(activity.Name)
	channelID, categoryGameID, categoryArchiveID := findChannelsID(name, channels)

	categoryGameID, err = createCategory(categoryGame, categoryGameID, client, event)

	if err != nil {
		slog.Error("cannot create category game", slog.Any("error", oops.Wrap(err)))

		return
	}

	categoryArchiveID, err = createCategory(categoryArchive, categoryArchiveID, client, event)

	if err != nil {
		slog.Error("cannot create category archive", slog.Any("error", oops.Wrap(err)))

		return
	}

	moveToCategory := categoryGameID

	if !hasEnoughActivityUsage {
		moveToCategory = categoryArchiveID
	}

	channelPosition := findPosition(name, moveToCategory, channels)
	if activity.UUID == "" {
		channelID, err = createChannel(name, channelID, channelPosition, moveToCategory, client, event)

		if err != nil {
			slog.Error("cannot create channel", slog.Any("error", oops.Wrap(err)))

			return
		}
	}

	var channelsToUpdate []discord.GuildChannelPositionUpdate

	channelsToUpdate = append(channelsToUpdate, discord.GuildChannelPositionUpdate{
		ID:       channelID,
		ParentID: &moveToCategory,
		Position: disgojson.NewNullablePtr(channelPosition),
	})

	channelsToUpdate = append(channelsToUpdate, calculateNewChannelPosition(channelPosition, moveToCategory, channels)...)

	err = client.UpdateChannelPositions(event.GuildID, channelsToUpdate)

	if err != nil {
		slog.Warn("cannot update channel positions", slog.Any("error", oops.Wrap(err)))

		return
	}
}

func findChannelsID(name string, channels []discord.GuildChannel) (snowflake.ID, snowflake.ID, snowflake.ID) {
	var channelID snowflake.ID

	var categoryArchiveID snowflake.ID

	var categoryGameID snowflake.ID

	for _, channel := range channels {
		if channel.Name() == name {
			channelID = channel.ID()
		}

		if channel.Type() != discord.ChannelTypeGuildCategory {
			continue
		}

		channelName := strings.ToLower(channel.Name())
		if channelName == categoryArchive {
			categoryArchiveID = channel.ID()
		}

		if channelName == categoryGame {
			categoryGameID = channel.ID()
		}

		if channelID > 0 && categoryArchiveID > 0 && categoryGameID > 0 {
			break
		}
	}

	return channelID, categoryGameID, categoryArchiveID
}

func findPosition(name string, category snowflake.ID, channels []discord.GuildChannel) int {
	position := 0

	for _, channel := range channels {
		if channel.ParentID() != nil && *channel.ParentID() == category && channel.Name() < name {
			position = channel.Position() + 1
		}
	}

	return position
}

func calculateNewChannelPosition(
	shiftPosition int,
	category snowflake.ID,
	channels []discord.GuildChannel,
) []discord.GuildChannelPositionUpdate {
	if shiftPosition == 0 {
		return []discord.GuildChannelPositionUpdate{}
	}

	var channelsToUpdate []discord.GuildChannelPositionUpdate

	for _, channel := range channels {
		if channel.ParentID() == nil {
			continue
		}

		if *channel.ParentID() != category {
			continue
		}

		if channel.Position() >= shiftPosition {
			channelsToUpdate = append(channelsToUpdate, discord.GuildChannelPositionUpdate{
				ID:       channel.ID(),
				Position: disgojson.NewNullablePtr(channel.Position() + 1),
			})
		}
	}

	return channelsToUpdate
}

func createCategory(
	categoryName string,
	category snowflake.ID,
	client rest.Rest,
	event *events.PresenceUpdate,
) (snowflake.ID, error) {
	if category != 0 {
		return category, nil
	}

	channel, err := client.CreateGuildChannel(event.GuildID, discord.GuildCategoryChannelCreate{
		Name: categoryName,
	})

	if err != nil {
		return 0, oops.Wrapf(err, "cannot create category")
	}

	return channel.ID(), nil
}

func createChannel(
	channelName string,
	channel snowflake.ID,
	channelPosition int,
	category snowflake.ID,
	client rest.Rest,
	event *events.PresenceUpdate,
) (snowflake.ID, error) {
	if channel != 0 {
		return channel, nil
	}

	if category == 0 {
		return channel, nil
	}

	guildChannel, err := client.CreateGuildChannel(event.GuildID, discord.GuildTextChannelCreate{
		Name:     channelName,
		ParentID: category,
		Position: channelPosition,
	})

	if err != nil {
		return 0, oops.Wrapf(err, "cannot create channel")
	}

	return guildChannel.ID(), nil
}
