package autochannelactivity

import (
	"context"
	"database/sql"

	"github.com/disgoorg/disgo/events"
	"github.com/disgoorg/snowflake/v2"
	_ "github.com/mattn/go-sqlite3"
	"github.com/samber/oops"
)

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

func (r *Repository) InsertStatement(ctx context.Context) (*sql.Stmt, error) {
	stmt, err := r.DB.PrepareContext(ctx, `INSERT INTO aca_activity
			(uuid, guild_id, user_id, activity_name, started_at) VALUES (?, ?, ?, ?, ?)`)
	if err != nil {
		return nil, oops.Wrapf(err, "failed to prepare statement")
	}

	return stmt, nil
}

func (r *Repository) CloseActivityStatement(ctx context.Context) (*sql.Stmt, error) {
	stmt, err := r.DB.PrepareContext(ctx, `UPDATE aca_activity 
		SET duration = 
			CAST(strftime('%s', CURRENT_TIMESTAMP) as integer) 
			- CAST(strftime('%s', aca_activity.started_at) as integer) 
		WHERE uuid = ?`)
	if err != nil {
		return nil, oops.Wrapf(err, "failed to prepare statement")
	}

	return stmt, nil
}

func (r *Repository) GetCurrentActivitiesUUID(
	ctx context.Context,
	event *events.PresenceUpdate,
) ([]CurrentActivity, error) {
	stmt, err := r.DB.PrepareContext(ctx, `SELECT uuid, activity_name 
		FROM aca_activity WHERE guild_id = ? AND user_id = ? AND duration = 0`)
	if err != nil {
		return nil, oops.Wrapf(err, "failed to prepare statement")
	}
	defer stmt.Close()

	rows, err := stmt.QueryContext(ctx, event.GuildID, event.PresenceUser.ID)
	if err != nil {
		return nil, oops.Wrapf(err, "failed to execute query")
	}
	defer rows.Close()

	var currentActivities []CurrentActivity

	for rows.Next() {
		var uuidv7 Uuidv7

		var name string

		err = rows.Scan(&uuidv7, &name)

		if err != nil {
			return nil, oops.Wrapf(err, "failed to scan row")
		}

		currentActivities = append(currentActivities, CurrentActivity{
			UUID: uuidv7,
			Name: name,
		})
	}

	err = rows.Err()

	if err != nil {
		return nil, oops.Wrapf(err, "failed to fetch rows")
	}

	return currentActivities, nil
}
