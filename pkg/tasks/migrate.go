package tasks

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/DECODEproject/iotencoder/pkg/logger"
	"github.com/DECODEproject/iotencoder/pkg/postgres"
	"github.com/DECODEproject/iotencoder/pkg/version"
)

func init() {
	rootCmd.AddCommand(migrateCmd)
	migrateCmd.AddCommand(migrateNewCmd)
	migrateCmd.AddCommand(migrateDownCmd)
	migrateCmd.AddCommand(migrateUpCmd)

	migrateNewCmd.Flags().String("dir", "pkg/migrations/sql", "The directory into which new migrations should be created")
	migrateDownCmd.Flags().IntP("steps", "s", 1, "Number of down migrations to run")
	migrateDownCmd.Flags().Bool("all", false, "Boolean flag that if true runs all down migrations")
}

var migrateCmd = &cobra.Command{
	Use:   "migrate",
	Short: "Manage Postgres migrations",
	Long: `This task provides subcommands for working with migrations for Postgres.

Up migrations are run automatically when the application boots, but here we
also offer commands to create properly named migration files, and a command
to run down migrations.`,
}

var migrateNewCmd = &cobra.Command{
	Use:   "new",
	Short: "Create a new Postgres migration",
	Long: fmt.Sprintf(`This command is a simple helper that creates a pair of matching migration
files, correctly named within the specified directory. The desired name of
the migration should be passed via a positional argument after the new
subcommand.

For example:

    $ %s migrate new AddUserTable`, version.BinaryName),
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		dir, err := cmd.Flags().GetString("dir")
		if err != nil {
			return err
		}

		logger := logger.NewLogger()

		return postgres.NewMigration(dir, args[0], logger)
	},
}

var migrateDownCmd = &cobra.Command{
	Use:   "down",
	Short: "Run down migrations against Postgres",
	Long: `This command can be used to rollback migrations executed against postgres. It
takes as parameters: the number of steps to rollback (default 1), or a
boolean flag (--all) indicating we should rollback all migrations. The
default is to simply rollback one migration.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		datasource, err := GetFromEnv(DatabaseURLKey)
		if err != nil {
			return err
		}

		steps, err := cmd.Flags().GetInt("steps")
		if err != nil {
			return err
		}

		all, err := cmd.Flags().GetBool("all")
		if err != nil {
			return err
		}

		logger := logger.NewLogger()

		db, err := postgres.Open(datasource)
		if err != nil {
			return err
		}

		if all {
			return postgres.MigrateDownAll(db.DB, logger)
		}

		return postgres.MigrateDown(db.DB, steps, logger)
	},
}

var migrateUpCmd = &cobra.Command{
	Use:   "up",
	Short: "Run up migrations against Postgres",
	Long: `This command can be used to run up migrations against Postgres. It is
primarily intended to be used in development when working on migrations as
once deployed the server automatically attempts to run all up migrations on
boot.
	`,
	RunE: func(cmd *cobra.Command, args []string) error {
		connStr, err := GetFromEnv(DatabaseURLKey)
		if err != nil {
			return err
		}

		logger := logger.NewLogger()

		db, err := postgres.Open(connStr)
		if err != nil {
			return err
		}

		return postgres.MigrateUp(db.DB, logger)
	},
}
