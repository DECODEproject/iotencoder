# retryable-registry-prometheus

This library is a tiny wrapper around prometheus's `MustRegister` function
that does a single round of trying to unregister and re-register before
panicking.

The reason for creating this library is that when starting DECODE components
we use a simple backoff retry wrapper when booting the server to deal with
waiting for the database to be ready. This sometimes panics because of
Prometheus metrics as the default provider immediately panics if we attempt
to register the same collector twice.

The hope with this library is that we will instead unregister the old
collector and register a new one.