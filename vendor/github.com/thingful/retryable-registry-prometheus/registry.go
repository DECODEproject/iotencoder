package registry

import "github.com/prometheus/client_golang/prometheus"

type Registry struct {
	*prometheus.Registry
}

var (
	defaultRegistry = NewRegistry()

	// DefaultRegisterer is an instance of our retryable registry
	DefaultRegisterer prometheus.Registerer = defaultRegistry
)

// NewRegistry returns an instance of our retryable registry. This is a registry
// that when we call `MustRegister` we try to register, if that fails we try to
// unregister, and re-register the collector, else we panic as before.
func NewRegistry() *Registry {
	r := prometheus.NewRegistry()
	return &Registry{
		Registry: r,
	}
}

// MustRegister is our override of the MustRegister implementation provided by
// the prometheus library. It attempts to do one round of unregistering and
// re-registering before panicking. This is to support the backoff retry loop we
// do at bootup for the DECODE components.
func (r *Registry) MustRegister(cs ...prometheus.Collector) {
	for _, c := range cs {
		err := prometheus.Register(c)
		if err != nil {
			if prometheus.Unregister(c) {
				prometheus.MustRegister(c)
			} else {
				panic(err)
			}
		}
	}
}

// MustRegister registers the provided collectors with the DefaultRegisterer and
// panics if any error occurs.
func MustRegister(cs ...prometheus.Collector) {
	DefaultRegisterer.MustRegister(cs...)
}
