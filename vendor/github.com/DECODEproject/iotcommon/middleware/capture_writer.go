package middleware

import (
	"net/http"
)

// captureWriter is a struct that wraps http.ResponseWriter to allow capturing and
// exposing the status code.
type captureWriter struct {
	http.ResponseWriter
	statusCode int
}

// WriteHeader captures the status code, and then calls the wrapped response
// writer method.
func (cw *captureWriter) WriteHeader(statusCode int) {
	cw.statusCode = statusCode
	cw.ResponseWriter.WriteHeader(statusCode)
}
