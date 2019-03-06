package logger

import (
	"os"

	"github.com/DECODEproject/iotencoder/pkg/version"
	kitlog "github.com/go-kit/kit/log"
)

// NewLogger is a simple helper function that returns a kitlog.Logger instance
// ready for use.
func NewLogger() kitlog.Logger {
	logger := kitlog.NewLogfmtLogger(kitlog.NewSyncWriter(os.Stdout))
	logger = kitlog.With(logger,
		"service", version.BinaryName,
		"ts", kitlog.DefaultTimestampUTC,
		"version", version.Version,
	)

	return logger
}
