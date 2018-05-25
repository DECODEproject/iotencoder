package version

import (
	"fmt"
	"runtime"
)

const (
	unknown = "UNKNOWN"
)

// BinaryName is an exported string exposing the name of the binary. Should be
// set during build.
var BinaryName = unknown

// Version is an exported version string which should be substituted with a
// real value during build.
var Version = unknown

// BuildDate is an exported variable containing the date at which the binary
// was built. This value should be substituted for a real value during build.
var BuildDate = unknown

// VersionString returns a formatted version string suitable for displaying to
// the user. This is a verbose version string including build date.
func VersionString() string {
	return fmt.Sprintf("%s (%s/%s). build date: %s", Version, runtime.GOOS, runtime.GOARCH, BuildDate)
}
