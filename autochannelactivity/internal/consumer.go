package internal

import (
	"context"
	"encoding/json"
	"log/slog"
	"os"
	"os/signal"

	"github.com/disgoorg/disgo/events"
	"github.com/nats-io/nats.go/jetstream"
	"github.com/samber/oops"

	"eggmech/core"
)

const Name = "autoChannelActivity"

var Subjects = []string{"activity.gaming"}

func Run(ctx context.Context, getenv func(string) string) error {
	ctx, cancel := signal.NotifyContext(ctx, os.Interrupt)
	defer cancel()

	nc, err := core.Connect(getenv("NATS_URL"), -1)

	if err != nil {
		return oops.Wrapf(err, "failed to connect to nats")
	}

	return core.ConsumeActivity(ctx, nc, Name, Subjects, func(msg jetstream.Msg) error {
		var event *events.PresenceUpdate
		err := json.Unmarshal(msg.Data(), &event)

		if err != nil {
			return oops.Wrapf(err, "failed to unmarshal presence update event")
		}

		slog.Info("received from durable consumer", slog.Any("event", event), slog.Any("subject", msg.Subject()))
		return nil
	})
}
