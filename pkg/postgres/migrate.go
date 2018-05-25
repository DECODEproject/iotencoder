package postgres

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"time"

	kitlog "github.com/go-kit/kit/log"
	"github.com/golang-migrate/migrate"
	"github.com/golang-migrate/migrate/database/postgres"
	bindata "github.com/golang-migrate/migrate/source/go-bindata"
	"github.com/pkg/errors"
	"github.com/serenize/snaker"

	"github.com/thingful/iotencoder/pkg/migrations"
)

// MigrateUp attempts to run all up migrations against Postgres. Migrations are
// loaded from a bindata generated module that is compiled into the binary. It
// takes as parameters an sql.DB instance, and a logger instance.
func MigrateUp(db *sql.DB, logger kitlog.Logger) error {
	logger.Log("msg", "migrating DB up")

	m, err := getMigrator(db, logger)
	if err != nil {
		return errors.Wrap(err, "failed to create migrator")
	}

	err = m.Up()
	if err != migrate.ErrNoChange {
		return err
	}

	return nil
}

// MigrateDown attempts to run down migrations against Postgres. It takes as
// parameters an sql.DB instance, the number of steps to run, and a logger
// instance. Migrations are loaded from a bindata generated module that is
// compiled into the binary.
func MigrateDown(db *sql.DB, steps int, logger kitlog.Logger) error {
	logger.Log("msg", "migrating DB down", "steps", steps)

	m, err := getMigrator(db, logger)
	if err != nil {
		return errors.Wrap(err, "failed to create migrator")
	}

	return m.Steps(-steps)
}

// MigrateDownAll attempts to run all down migrations against Postgres. It takes
// as parameters an sql.DB instance, and a logger instance. Migrations are
// loaded from a bindata generated module that is compiled into the binary.
func MigrateDownAll(db *sql.DB, logger kitlog.Logger) error {
	logger.Log("msg", "migrating DB down all")

	m, err := getMigrator(db, logger)
	if err != nil {
		return errors.Wrap(err, "failed to create migrator")
	}

	return m.Down()
}

// NewMigration creates a new pair of files into which an SQL migration should
// be written. All this is doing is ensuring files created are correctly named.
func NewMigration(dirName, migrationName string, logger kitlog.Logger) error {
	if migrationName == "" {
		return errors.New("Must specify a name when creating a migration")
	}

	re := regexp.MustCompile(`\A[a-zA-Z]+\z`)
	if !re.MatchString(migrationName) {
		return errors.New("Name must be a single CamelCased string with no numbers or special characters")
	}

	migrationID := time.Now().Format("20060102150405") + "_" + snaker.CamelToSnake(migrationName)
	upFileName := fmt.Sprintf("%s.up.sql", migrationID)
	downFileName := fmt.Sprintf("%s.down.sql", migrationID)

	logger.Log("upfile", upFileName, "downfile", downFileName, "directory", dirName, "msg", "creating migration files")

	err := os.MkdirAll(dirName, 0755)
	if err != nil {
		return errors.Wrap(err, "failed to make directory for migrations")
	}

	upFile, err := os.Create(filepath.Join(dirName, upFileName))
	if err != nil {
		return errors.Wrap(err, "failed to make up migration file")
	}
	defer upFile.Close()

	downFile, err := os.Create(filepath.Join(dirName, downFileName))
	if err != nil {
		return errors.Wrap(err, "failed to make down migration file")
	}
	defer downFile.Close()

	return nil
}

// getMigrator instantiates and returns a migrate.Migrate instance, which we use
// to execute migrations against a database. It takes as parameters an sql.DB
// instance, and a logger. Migration data is loaded from a bindata generated
// module compiled into the binary.
func getMigrator(db *sql.DB, logger kitlog.Logger) (*migrate.Migrate, error) {
	dbDriver, err := postgres.WithInstance(db, &postgres.Config{})
	if err != nil {
		return nil, err
	}

	source := bindata.Resource(migrations.AssetNames(),
		func(name string) ([]byte, error) {
			return migrations.Asset(name)
		},
	)

	sourceDriver, err := bindata.WithInstance(source)
	if err != nil {
		return nil, err
	}

	migrator, err := migrate.NewWithInstance(
		"go-bindata",
		sourceDriver,
		"postgres",
		dbDriver,
	)
	if err != nil {
		return nil, err
	}

	migrator.Log = newLogAdapter(logger, true)

	return migrator, nil
}

// newLogAdapter simply wraps our gokit logger into our logAdapter type which
// allows it to be used by go-migrate.
func newLogAdapter(logger kitlog.Logger, verbose bool) migrate.Logger {
	return &logAdapter{logger: logger, verbose: verbose}
}

// logAdapter is a simple type we use to wrap the go-kit Logger to make it
// adhere to go-migrate's Logger interface.
type logAdapter struct {
	logger  kitlog.Logger
	verbose bool
}

// Printf is semantically the same as fmt.Printf. Here we simply output the
// result of fmt.Sprintf as the value of a `msg` key.
func (l *logAdapter) Printf(format string, v ...interface{}) {
	l.logger.Log("msg", fmt.Sprintf(format, v...))
}

// Verbose returns true when verbose logging output is wanted
func (l *logAdapter) Verbose() bool {
	return l.verbose
}
