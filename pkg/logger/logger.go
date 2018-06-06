package logger

import (
	"os"

	kitlog "github.com/go-kit/kit/log"
	"github.com/thingful/iotencoder/pkg/version"
)

// NewLogger is a simple helper function that returns a kitlog.Logger instance
// ready for use.
func NewLogger() kitlog.Logger {
	logger := kitlog.NewJSONLogger(kitlog.NewSyncWriter(os.Stdout))
	logger = kitlog.With(logger,
		"service", version.BinaryName,
		"ts", kitlog.DefaultTimestampUTC,
		"version", version.Version,
	)

	logger.Log("module", "logger", "msg", "creating logger instance")

	return logger
}
