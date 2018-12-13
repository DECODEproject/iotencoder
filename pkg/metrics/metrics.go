package metrics

import "github.com/prometheus/client_golang/prometheus"

// MustRegister is a simple wrapper around prometheus's built in MustRegister
// which makes an attempt to handle the case that a collector has already been
// registered. This can happen because of the retry loop we added at our main
// entrypoint where we attempt to loop on startup till the db is ready.
func MustRegister(c prometheus.Collector) {
	err := prometheus.Register(c)
	if err != nil {
		if prometheus.Unregister(c) {
			prometheus.MustRegister(c)
		} else {
			panic(err)
		}
	}
}
