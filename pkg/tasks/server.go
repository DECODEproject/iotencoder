package tasks

import (
	"github.com/spf13/cobra"

	"github.com/thingful/iotencoder/pkg/logger"
	"github.com/thingful/iotencoder/pkg/server"
)

func init() {
	rootCmd.AddCommand(serverCmd)

	serverCmd.Flags().StringP("addr", "a", "0.0.0.0:8080", "Specify the address to which the server binds")
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
		addr, err := cmd.Flags().GetString("addr")
		if err != nil {
			return err
		}

		datasource, err := GetFromEnv("IOTENCODER_DATABASE_URL")
		if err != nil {
			return err
		}

		logger := logger.NewLogger()

		s := server.NewServer(addr, datasource, logger)

		return s.Start()
	},
}
