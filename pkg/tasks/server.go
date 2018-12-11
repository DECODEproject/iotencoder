package tasks

import (
	"context"
	"errors"
	"time"

	"github.com/lestrrat-go/backoff"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/DECODEproject/iotencoder/pkg/logger"
	"github.com/DECODEproject/iotencoder/pkg/server"
)

func init() {
	rootCmd.AddCommand(serverCmd)
	serverCmd.Flags().StringP("addr", "a", "0.0.0.0:8081", "Address to which the HTTP server binds")
	serverCmd.Flags().StringP("datastore", "d", "", "Address at which the datastore is listening")
	serverCmd.Flags().String("database-url", "", "URL at which Postgres is listening (e.g. postgres://username:password@host:5432/dbname?sslmode=enable)")
	serverCmd.Flags().IntP("hashidlength", "l", 8, "Minimum length of generated hashids")
	serverCmd.Flags().Bool("verbose", false, "Enable verbose output")
	serverCmd.Flags().StringP("broker-addr", "b", "tcp://mqtt.smartcitizen.me:1883", "Address at which the MQTT broker is listening")
	serverCmd.Flags().StringP("redis-url", "r", "", "URL at which redis is listening (e.g. redis://password@host:6379/1)")

	viper.BindPFlag("addr", serverCmd.Flags().Lookup("addr"))
	viper.BindPFlag("datastore", serverCmd.Flags().Lookup("datastore"))
	viper.BindPFlag("hashidlength", serverCmd.Flags().Lookup("hashidlength"))
	viper.BindPFlag("verbose", serverCmd.Flags().Lookup("verbose"))
	viper.BindPFlag("broker-addr", serverCmd.Flags().Lookup("broker-addr"))
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

		connStr := viper.GetString("database-url")
		if connStr == "" {
			return errors.New("Missing required environment variable: $IOTENCODER_DATABASE_URL")
		}

		encryptionPassword := viper.GetString("encryption-password")
		if encryptionPassword == "" {
			return errors.New("Missing required environment variable: $IOTENCODER_ENCRYPTION_PASSWORD")
		}

		hashidSalt := viper.GetString("hashid_salt")
		if hashidSalt == "" {
			return errors.New("Missing required environment variable: $IOTENCODER_HASHID_SALT")
		}

		brokerAddr := viper.GetString("broker-addr")
		if brokerAddr == "" {
			return errors.New("Must provide MQTT broker address")
		}

		redisURL := viper.GetString("redis-url")
		if redisURL == "" {
			return errors.New("Must provide Redis URL, either via flag or environment variable")
		}

		logger := logger.NewLogger()

		config := &server.Config{
			ListenAddr:         addr,
			DatastoreAddr:      datastoreAddr,
			ConnStr:            connStr,
			EncryptionPassword: encryptionPassword,
			HashidSalt:         hashidSalt,
			HashidMinLength:    viper.GetInt("hashidlength"),
			Verbose:            viper.GetBool("verbose"),
			BrokerAddr:         brokerAddr,
			RedisURL:           redisURL,
		}

		executer := backoff.ExecuteFunc(func(_ context.Context) error {
			s := server.NewServer(config, logger)
			return s.Start()
		})

		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
		defer cancel()

		policy := backoff.NewExponential()
		return backoff.Retry(ctx, policy, executer)
	},
}
