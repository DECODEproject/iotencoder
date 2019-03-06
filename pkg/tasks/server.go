package tasks

import (
	"context"
	"errors"
	"time"

	raven "github.com/getsentry/raven-go"
	"github.com/lestrrat-go/backoff"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/DECODEproject/iotencoder/pkg/logger"
	"github.com/DECODEproject/iotencoder/pkg/server"
	"github.com/DECODEproject/iotencoder/pkg/version"
)

func init() {
	rootCmd.AddCommand(serverCmd)
	serverCmd.Flags().StringP("addr", "a", ":8081", "Address to which the HTTP server binds")
	serverCmd.Flags().StringP("datastore", "d", "", "Address at which the datastore is listening")
	serverCmd.Flags().String("database-url", "", "URL at which Postgres is listening (e.g. postgres://username:password@host:5432/dbname?sslmode=enable)")
	serverCmd.Flags().String("encryption-password", "", "Password used to encrypt secret tokens we write to Postgres")
	serverCmd.Flags().IntP("hashid-length", "l", 8, "Minimum length of generated ids for streams")
	serverCmd.Flags().String("hashid-salt", "", "Salt value used when hashing generated ids for streams")
	serverCmd.Flags().Bool("verbose", false, "Enable verbose output")
	serverCmd.Flags().StringP("broker-addr", "b", "tcps://mqtt.smartcitizen.me:8883", "Address at which the MQTT broker is listening")
	serverCmd.Flags().StringP("broker-username", "u", "", "Username for accessing the MQTT broker")
	serverCmd.Flags().StringP("redis-url", "r", "", "URL at which redis is listening (e.g. redis://password@host:6379/1)")
	serverCmd.Flags().StringSlice("domains", []string{}, "Comma separated list of domains to enable TLS for these domains")

	viper.BindPFlag("addr", serverCmd.Flags().Lookup("addr"))
	viper.BindPFlag("datastore", serverCmd.Flags().Lookup("datastore"))
	viper.BindPFlag("database-url", serverCmd.Flags().Lookup("database-url"))
	viper.BindPFlag("encryption-password", serverCmd.Flags().Lookup("encryption-password"))
	viper.BindPFlag("hashid-length", serverCmd.Flags().Lookup("hashid-length"))
	viper.BindPFlag("hashid-salt", serverCmd.Flags().Lookup("hashid-salt"))
	viper.BindPFlag("verbose", serverCmd.Flags().Lookup("verbose"))
	viper.BindPFlag("broker-addr", serverCmd.Flags().Lookup("broker-addr"))
	viper.BindPFlag("broker-username", serverCmd.Flags().Lookup("broker-username"))
	viper.BindPFlag("redis-url", serverCmd.Flags().Lookup("redis-url"))
	viper.BindPFlag("domains", serverCmd.Flags().Lookup("domains"))

	raven.SetRelease(version.Version)
	raven.SetTagsContext(map[string]string{"component": "encoder"})
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
clients unable to use the Protocol Buffer API.

Configuration values can be provided either by flags, or generally by
environment variables. If a flag is named: --example-flag, then it will also be
able to be supplied via an environment variable: $IOTENCODER_EXAMPLE_FLAG`,
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
			return errors.New("Must provide postgres database url")
		}

		encryptionPassword := viper.GetString("encryption-password")
		if encryptionPassword == "" {
			return errors.New("Must provide postgres encryption password")
		}

		hashidSalt := viper.GetString("hashid-salt")
		if hashidSalt == "" {
			return errors.New("Missing required environment variable: $IOTENCODER_HASHID_SALT")
		}

		brokerAddr := viper.GetString("broker-addr")
		if brokerAddr == "" {
			return errors.New("Must provide MQTT broker address to which updates are published")
		}

		brokerUsername := viper.GetString("broker-username")
		if brokerUsername == "" {
			return errors.New("Must provide MQTT broker username to authenticate access to the broker")
		}

		redisURL := viper.GetString("redis-url")
		if redisURL == "" {
			return errors.New("Must provide redis url")
		}

		logger := logger.NewLogger()

		config := &server.Config{
			ListenAddr:         addr,
			DatastoreAddr:      datastoreAddr,
			ConnStr:            connStr,
			EncryptionPassword: encryptionPassword,
			HashidSalt:         hashidSalt,
			HashidMinLength:    viper.GetInt("hashid-length"),
			Verbose:            viper.GetBool("verbose"),
			BrokerAddr:         brokerAddr,
			BrokerUsername:     brokerUsername,
			RedisURL:           redisURL,
			Domains:            viper.GetStringSlice("domains"),
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
