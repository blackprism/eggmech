package autochannelactivity

import (
	"embed"
	"log/slog"
	"net/url"

	"github.com/amacneil/dbmate/v2/pkg/dbmate"
	_ "github.com/amacneil/dbmate/v2/pkg/driver/sqlite"
	"github.com/samber/oops"
)

func migration(migrationsEmbed embed.FS) error {
	databaseURL, _ := url.Parse("sqlite:deployments/data/database.sqlite3")
	db := dbmate.New(databaseURL)
	db.MigrationsDir = []string{"migrations"}
	db.FS = migrationsEmbed
	db.SchemaFile = "deployments/data/database-schema.sql"

	files, _ := migrationsEmbed.ReadDir("migrations")

	if len(files) == 1 && files[0].Name() == "blank.sql" {
		return nil
	}

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
