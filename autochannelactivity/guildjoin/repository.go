package guildjoin

import (
	"context"
	"database/sql"

	"github.com/disgoorg/disgo/events"
	"github.com/disgoorg/snowflake/v2"
	"github.com/gofrs/uuid/v5"
	_ "github.com/mattn/go-sqlite3"
	"github.com/samber/oops"
)

const MinimumPlayers = 1
const MinimumHours = 1
const DayInterval = 1

type Uuidv7 string
type GuildID snowflake.ID
type UserID snowflake.ID

type CurrentActivity struct {
	UUID Uuidv7
	Name string
}

type Repository struct {
	DB *sql.DB
}

func (r *Repository) SetDefaultSettings(
	ctx context.Context,
	event *events.GuildJoin,
) error {
	stmt, err := r.DB.PrepareContext(
		ctx,
		`INSERT INTO aca_activity_settings (uuid, guild_id, minimum_players, minimum_hours, day_interval) VALUES (?, ?, ?, ?, ?)`,
	)

	if err != nil {
		return oops.Wrapf(err, "failed to prepare statement")
	}
	defer stmt.Close()

	uuidv7, err := uuid.NewV7()

	_, err = stmt.ExecContext(ctx, uuidv7, event.GuildID, MinimumPlayers, MinimumHours, DayInterval)
	if err != nil {
		return oops.Wrapf(err, "failed to execute query")
	}

	return nil
}
