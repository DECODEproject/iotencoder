package tasks

import (
	"log"
	"strings"

	raven "github.com/getsentry/raven-go"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/DECODEproject/iotencoder/pkg/version"
)

func init() {
	viper.SetEnvPrefix("iotencoder")
	viper.AutomaticEnv()
	replacer := strings.NewReplacer("-", "_")
	viper.SetEnvKeyReplacer(replacer)
}

var rootCmd = &cobra.Command{
	Use:   version.BinaryName,
	Short: "Encrypted datastore for the DECODE IoT Pilot",
	Long: `This tool is an implementation of the encrypted datastore interface being
developed as part of the IoT Pilot for DECODE (https://decodeproject.eu/).

This component exposes a simple RPC API implemented using a library called
Twirp, that provides either a JSON or Protocol Buffer API over HTTP 1.1.

Data is currently persisted to PostgreSQL, and for the purposes of
demonstration we assume that we are able to trust callers to service. All
data stored within the datastore is encrypted for a specific target client,
meaning this datastore has no visibility of the data being persisted.
`,
	Version: version.VersionString(),
}

// Execute is our main entrypoint to the application
func Execute() {
	if err := rootCmd.Execute(); err != nil {
		raven.CaptureErrorAndWait(err, nil)
		log.Fatal(err)
	}
}
