package middleware

import (
	"context"
	"net/http"

	"github.com/google/uuid"
)

type contextKey string

const (
	// RequestIDHeader is a constant containing the request header key
	RequestIDHeader = "X-Request-ID"

	// RequestCtxKey is a context containing the context key under which the
	// request ID is stored.
	RequestCtxKey = contextKey("requestID")
)

// RequestIDMiddleware is a net.http middleware that adds a unique UUID request ID
// to requests. If a request ID is already present in the request we use that
// one else we generate a random UUID. The request ID is added to our context so
// will be available within any downstream handlers.
func RequestIDMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rid := r.Header.Get(RequestIDHeader)
		if rid == "" {
			rid = uuid.New().String()
		}
		w.Header().Set(RequestIDHeader, rid)
		ctx := context.WithValue(r.Context(), RequestCtxKey, rid)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}
