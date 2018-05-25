package tasks

import (
	"fmt"
	"os"
)

// GetFromEnv is a simple wrapper around os.Getenv that emits an error if a
// variable is not present in the environment.
func GetFromEnv(key string) (string, error) {
	val := os.Getenv(key)
	if val == "" {
		return "", fmt.Errorf("Missing required environment variable: $%s", key)
	}

	return val, nil
}
