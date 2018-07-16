package tasks

import (
	"errors"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/thingful/iotencoder/pkg/logger"
	"github.com/thingful/iotencoder/pkg/server"
)

func init() {
	rootCmd.AddCommand(serverCmd)
	serverCmd.Flags().StringP("addr", "a", "0.0.0.0:8081", "Address to which the HTTP server binds")
	serverCmd.Flags().StringP("datastore", "d", "", "Address at which the datastore is listening")
	serverCmd.Flags().IntP("hashidlength", "l", 8, "Minimum length of generated hashids")
	serverCmd.Flags().Bool("verbose", false, "Enable verbose output")

	viper.BindPFlag("addr", serverCmd.Flags().Lookup("addr"))
	viper.BindPFlag("datastore", serverCmd.Flags().Lookup("datastore"))
	viper.BindPFlag("hashidlength", serverCmd.Flags().Lookup("hashidlength"))
	viper.BindPFlag("verbose", serverCmd.Flags().Lookup("verbose"))
}

var serverCmd = &cobra.Command{
	Use:   "server",
	Short: "Starts datastore listening for requests",
	Long: `
Starts our implementation of the DECODE datastore RPC interface, which is
designed to expose a simple API to store and retrieve encrypted events coming
from upstream IoT devices.

The server uses Twirp to expose both a JSON API along with a more performant
Protocol Buffer API. The JSON API is not intended for use other than for
clients unable to use the Protocol Buffer API.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		addr := viper.GetString("addr")
		if addr == "" {
			return errors.New("Must provide a bind address")
		}

		datastoreAddr := viper.GetString("datastore")
		if datastoreAddr == "" {
			return errors.New("Must provide datastore address")
		}

		connStr := viper.GetString("database_url")
		if connStr == "" {
			return errors.New("Missing required environment variable: $IOTENCODER_DATABASE_URL")
		}

		encryptionPassword := viper.GetString("encryption_password")
		if encryptionPassword == "" {
			return errors.New("Missing required environment variable: $IOTENCODER_ENCRYPTION_PASSWORD")
		}

		hashidSalt := viper.GetString("hashid_salt")
		if hashidSalt == "" {
			return errors.New("Missing required environment variable: $IOTENCODER_HASHID_SALT")
		}

		verbose := viper.GetBool("verbose")

		logger := logger.NewLogger()

		config := &server.Config{
			ListenAddr:         addr,
			DatastoreAddr:      datastoreAddr,
			ConnStr:            connStr,
			EncryptionPassword: encryptionPassword,
			HashidSalt:         hashidSalt,
			HashidMinLength:    viper.GetInt("hashidlength"),
			Verbose:            verbose,
		}

		s, err := server.NewServer(config, logger)
		if err != nil {
			return err
		}

		return s.Start()
	},
}
