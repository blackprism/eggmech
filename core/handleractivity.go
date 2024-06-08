package core

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/disgo/events"
	"github.com/disgoorg/snowflake/v2"
	"github.com/nats-io/nats.go/jetstream"
	"github.com/samber/oops"
)

func PresenceHandler(
	ctx context.Context,
	jsConn jetstream.JetStream,
	botID snowflake.ID,
	subject string,
) func(event *events.PresenceUpdate) {
	return func(event *events.PresenceUpdate) {
		if botID == event.PresenceUser.ID {
			return
		}

		activities := [...]string{"gaming", "streaming", "listening", "watching", "custom", "competing"}

		activitiesByType := make(map[string][]discord.Activity)
		for _, activity := range event.Activities {
			activitiesByType[activities[activity.Type]] = append(activitiesByType[activities[activity.Type]], activity)
		}

		for _, activity := range activities {
			subjectActivity := fmt.Sprintf("%s.%s", subject, activity)
			slog.Info("subjectActivity is...", slog.String("subjectActivity", subjectActivity))

			event.Activities = activitiesByType[activity]
			eventPublishable, err := json.Marshal(event)

			if err != nil {
				slog.Error(
					"event not publishable",
					slog.String("event", "activityStart"),
					slog.Any("error", oops.Wrap(err)),
					slog.Any("payload", event),
				)

				continue
			}

			_, errPublish := jsConn.Publish(ctx, subjectActivity, eventPublishable)

			if errPublish != nil {
				slog.Error(
					"event not published",
					slog.String("event", "activityStart"),
					slog.Any("error", oops.Wrap(errPublish)),
					slog.Any("payload", eventPublishable),
				)
			}
		}
	}
}
