package tasks

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/thingful/iotencoder/pkg/version"
)

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

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}
