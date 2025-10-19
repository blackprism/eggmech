package activity

import (
	"context"
	"database/sql"
	"errors"

	"github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/disgo/events"
	"github.com/disgoorg/snowflake/v2"
	"github.com/gofrs/uuid/v5"
	_ "github.com/mattn/go-sqlite3" // SQLite driver
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
	db         *sql.DB
	Statements map[string]*sql.Stmt
}

func BuildRepository(db *sql.DB) Repository {
	return Repository{
		db:         db,
		Statements: make(map[string]*sql.Stmt),
	}
}

func (r *Repository) Close() {
	for _, stmt := range r.Statements {
		_ = stmt.Close()
	}
}

func (r *Repository) getStatement(ctx context.Context, name string, query string) (*sql.Stmt, error) {
	if r.Statements[name] == nil {
		stmt, err := r.db.PrepareContext(ctx, query)
		if err != nil {
			return nil, oops.Wrapf(err, "failed to prepare statement")
		}

		r.Statements[name] = stmt
	}

	return r.Statements[name], nil
}

func (r *Repository) InsertStatement(ctx context.Context) (*sql.Stmt, error) {
	stmt, err := r.db.PrepareContext(ctx, `INSERT INTO aca_activity
			(uuid, guild_id, user_id, activity_name, started_at) VALUES (?, ?, ?, ?, ?)`)

	if err != nil {
		return nil, oops.Wrapf(err, "failed to prepare statement")
	}

	return stmt, nil
}

func (r *Repository) CloseActivityStatement(ctx context.Context) (*sql.Stmt, error) {
	stmt, err := r.db.PrepareContext(ctx, `UPDATE aca_activity 
		SET duration = 
			CAST(strftime('%s', CURRENT_TIMESTAMP) as integer) 
			- CAST(strftime('%s', aca_activity.started_at) as integer) 
		WHERE uuid = ?`)
	if err != nil {
		return nil, oops.Wrapf(err, "failed to prepare statement")
	}

	return stmt, nil
}

func (r *Repository) InsertActivity(
	ctx context.Context,
	event *events.PresenceUpdate,
	activity discord.Activity,
) error {
	//nolint:sqlclosecheck // statement pool
	stmt, errStmt := r.getStatement(ctx, "insert_activity", `INSERT INTO aca_activity
			(uuid, guild_id, user_id, activity_name, started_at) VALUES (?, ?, ?, ?, ?)`)

	if errStmt != nil {
		return oops.Wrapf(errStmt, "can't get statement for insert activity")
	}

	uuidv7, errUUID := uuid.NewV7()
	if errUUID != nil {
		return oops.Wrapf(errUUID, "failed to generate uuid for insert activity")
	}

	_, errExec := stmt.ExecContext(
		ctx,
		uuidv7,
		event.GuildID,
		event.PresenceUser.ID,
		activity.Name,
		activity.CreatedAt,
	)

	if errExec != nil {
		return oops.Wrapf(errExec, "can't insert activity")
	}

	return nil
}

func (r *Repository) CreateChannel(
	ctx context.Context,
	guildID snowflake.ID,
	channelID snowflake.ID,
	channel string,
) error {
	//nolint:sqlclosecheck // statement pool
	stmtGet, errStmtGet := r.getStatement(
		ctx,
		"get_activity_settings",
		`SELECT uuid FROM aca_activity_settings WHERE guild_id = ? AND channel_id = ?`,
	)

	if errStmtGet != nil {
		return oops.Wrapf(errStmtGet, "can't get statement for get activity settings")
	}

	row := stmtGet.QueryRowContext(
		ctx,
		guildID,
		channelID,
	)

	var uuidv7ForSettings Uuidv7
	errScan := row.Scan(&uuidv7ForSettings)

	if errScan != nil && !errors.Is(errScan, sql.ErrNoRows) {
		return oops.Wrapf(errScan, "can't get uuid for channel")
	}

	if uuidv7ForSettings == "" {
		//nolint:sqlclosecheck // statement pool
		stmt, errStmt := r.getStatement(
			ctx,
			"insert_activity_settings",
			`INSERT INTO aca_activity_settings
			(uuid, guild_id, channel_id, minimum_players, minimum_hours, day_interval) 
			VALUES (?, ?, ?, ?, ?, ?)`,
		)

		if errStmt != nil {
			return oops.Wrapf(errStmt, "can't get statement for insert activity settings")
		}

		uuidv7, errUUID := uuid.NewV7()
		if errUUID != nil {
			return oops.Wrapf(errUUID, "failed to generate uuid for insert activity settings")
		}
		uuidv7ForSettings = Uuidv7(uuidv7.String())

		_, errExec := stmt.ExecContext(
			ctx,
			uuidv7ForSettings,
			guildID,
			channelID,
			MinimumPlayers,
			MinimumHours,
			DayInterval,
		)

		if errExec != nil {
			return oops.Wrapf(errExec, "can't insert activity settings")
		}
	}

	//nolint:sqlclosecheck // statement pool
	stmt, errStmt := r.getStatement(
		ctx,
		"insert_activity_channel",
		`INSERT INTO aca_activity_channel
			(uuid, activity_settings_uuid, activity_name) 
			VALUES (?, ?, ?)`,
	)

	if errStmt != nil {
		return oops.Wrapf(errStmt, "can't get statement for insert activity channel")
	}

	uuidv7ForChannel, errUUID := uuid.NewV7()
	if errUUID != nil {
		return oops.Wrapf(errUUID, "failed to generate uuid for insert activity channel")
	}

	_, errExec := stmt.ExecContext(
		ctx,
		uuidv7ForChannel,
		uuidv7ForSettings,
		channel,
	)

	if errExec != nil {
		return oops.Wrapf(errExec, "can't insert activity channel")
	}

	return nil
}

func (r *Repository) GetCurrentActivitiesUUID(
	ctx context.Context,
	event *events.PresenceUpdate,
) ([]CurrentActivity, error) {
	stmt, err := r.db.PrepareContext(ctx, `SELECT uuid, activity_name 
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

func (r *Repository) HasEnoughActivityUsage(ctx context.Context, activityName string) (bool, error) {
	stmt, errPrepare := r.db.PrepareContext(ctx, `WITH activities AS (
    SELECT SUM(duration) AS duration,
           count(distinct user_id) AS players,
           CASE WHEN aas.uuid IS NOT NULL THEN aas.minimum_hours ELSE ? END * 1 AS minimum_seconds,
           CASE WHEN aas.uuid IS NOT NULL THEN aas.minimum_players ELSE ? END AS minimum_players
    FROM aca_activity aa
    LEFT JOIN aca_activity_settings AS aas
        ON (aas.guild_id = aa.guild_id)
    LEFT JOIN aca_activity_channel AS aac ON (aac.activity_settings_uuid = aas.uuid AND aac.activity_name = aa.activity_name)
    WHERE
        aa.started_at > DATE(
                'now',
                '-' || CASE WHEN aas.uuid IS NOT NULL THEN aas.day_interval ELSE ? END || ' day'
                        )
      AND aa.activity_name = ?
    GROUP BY aa.activity_name)
SELECT 1 FROM activities WHERE players >= minimum_players AND duration >= minimum_seconds`)
	if errPrepare != nil {
		return false, oops.Wrapf(errPrepare, "failed to prepare statement")
	}
	defer stmt.Close()

	rows, errQuery := stmt.QueryContext(ctx, MinimumPlayers, MinimumHours, DayInterval, activityName)
	if errQuery != nil {
		return false, oops.Wrapf(errQuery, "failed to execute query")
	}
	defer rows.Close()

	if rows.Err() != nil {
		return false, oops.Wrapf(rows.Err(), "failed to fetch rows")
	}

	hasRow := rows.Next()

	if !hasRow {
		return false, nil
	}

	var hasEnough uint

	errScan := rows.Scan(&hasEnough)

	if errScan != nil {
		return false, oops.Wrapf(rows.Err(), "failed to scan row")
	}

	return true, nil
}
