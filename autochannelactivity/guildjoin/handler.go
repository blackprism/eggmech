package guildjoin

import (
	"context"
	"log/slog"

	"github.com/disgoorg/disgo/events"
	"github.com/samber/oops"
)

func GuildJoinHandler(
	ctx context.Context,
	repo Repository,
) func(event *events.GuildJoin) {
	return func(event *events.GuildJoin) {

		errHandler := handler(ctx, repo, event)

		if errHandler != nil {
			slog.Error("failed to run handler", slog.Any("error", errHandler))

			return
		}
	}
}

func handler(
	ctx context.Context,
	repo Repository,
	event *events.GuildJoin,
) error {
	err := repo.SetDefaultSettings(ctx, event)

	if err != nil {
		return oops.Wrapf(err, "failed to set default settings")
	}

	return nil
}
