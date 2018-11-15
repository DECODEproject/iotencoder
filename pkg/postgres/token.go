package postgres

import (
	"crypto/rand"
	"encoding/base64"
)

// GenerateToken returns a cryptographically secure base64 encoded random string
// generated using crypto/rand.
func GenerateToken(n int) (string, error) {
	b, err := generateRandomBytes(n)
	if err != nil {
		return "", err
	}

	return base64.StdEncoding.EncodeToString(b), nil
}

// generateRandomBytes returns a byte array containing cryptographically secure
// random data generated using crypto/rand.
func generateRandomBytes(n int) ([]byte, error) {
	b := make([]byte, n)
	_, err := rand.Read(b)
	if err != nil {
		return nil, err
	}

	return b, nil
}
