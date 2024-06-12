package autochannelactivity

import (
	"context"
	"database/sql"
	"encoding/json"
	"log/slog"
	"os"
	"os/signal"

	"github.com/disgoorg/disgo/events"
	"github.com/gofrs/uuid/v5"
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

	handler := func(insertStatement *sql.Stmt, closeActivityStatement *sql.Stmt) func(msg jetstream.Msg) error {
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

			processActivitiesToClose(ctx, closeActivityStatement, event, currentActivities)
			processActivitiesToCreate(ctx, insertStatement, event, currentActivities)

			return nil
		}
	}

	return core.ConsumeActivity(
		ctx,
		natsConn,
		Name,
		[]string{"activity.gaming"},
		handler(insertStatement, closeActivityStatement),
	)
}

func processActivitiesToClose(
	ctx context.Context,
	closeActivityStatement *sql.Stmt,
	event *events.PresenceUpdate,
	currentActivities []CurrentActivity,
) {
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
		}
	}
}

func processActivitiesToCreate(
	ctx context.Context,
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
		}
	}
}
