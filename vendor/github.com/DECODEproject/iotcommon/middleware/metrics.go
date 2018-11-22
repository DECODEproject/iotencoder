package middleware

import (
	"net/http"
	"strconv"
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

// metricsMiddleware is our middleware implementation, defined as a struct as it
// has to hold the state of the configured prometheus HistogramVec.
type metricsMiddleware struct {
	h        http.Handler
	duration *prometheus.HistogramVec
}

// ServeHTTP is our implementation of the http.Handler interface
func (m *metricsMiddleware) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	cw := &captureWriter{
		ResponseWriter: w,
		statusCode:     http.StatusOK,
	}
	startTime := time.Now()
	m.h.ServeHTTP(cw, r)
	took := time.Since(startTime)
	m.duration.WithLabelValues(
		strconv.Itoa(cw.statusCode), r.Method, r.URL.Path).
		Observe(took.Seconds())
}

// MetricsMiddleware returns a new middleware that can then be Used by goji, or
// any other standard http.Handler based server.
func MetricsMiddleware(namespace, subsystem string) func(http.Handler) http.Handler {
	duration := prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: namespace,
			Subsystem: subsystem,
			Name:      "request_duration_sec",
			Help:      "Time (in seconds) spent serving HTTP requests",
			Buckets:   prometheus.DefBuckets,
		}, []string{"status_code", "method", "path"},
	)

	prometheus.MustRegister(duration)

	fn := func(h http.Handler) http.Handler {
		return &metricsMiddleware{
			h:        h,
			duration: duration,
		}
	}

	return fn
}
