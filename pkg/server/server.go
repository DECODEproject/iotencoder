package server

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"time"

	"github.com/DECODEproject/iotcommon/middleware"
	kitlog "github.com/go-kit/kit/log"
	twrpprom "github.com/joneskoo/twirp-serverhook-prometheus"
	"github.com/pkg/errors"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	datastore "github.com/thingful/twirp-datastore-go"
	encoder "github.com/thingful/twirp-encoder-go"
	goji "goji.io"
	"goji.io/pat"

	"github.com/DECODEproject/iotencoder/pkg/metrics"
	"github.com/DECODEproject/iotencoder/pkg/mqtt"
	"github.com/DECODEproject/iotencoder/pkg/pipeline"
	"github.com/DECODEproject/iotencoder/pkg/postgres"
	"github.com/DECODEproject/iotencoder/pkg/redis"
	"github.com/DECODEproject/iotencoder/pkg/rpc"
	"github.com/DECODEproject/iotencoder/pkg/system"
	"github.com/DECODEproject/iotencoder/pkg/version"
)

var (
	buildInfo = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: "decode",
			Subsystem: "encoder",
			Name:      "build_info",
			Help:      "Information about the current build of the service",
		}, []string{"name", "version", "build_date"},
	)
)

func init() {
	metrics.MustRegister(buildInfo)
}

// Config is a top level config object. Populated by viper in the command setup,
// we then pass down config to the right places.
type Config struct {
	ListenAddr         string
	ConnStr            string
	EncryptionPassword string
	HashidSalt         string
	HashidMinLength    int
	DatastoreAddr      string
	Verbose            bool
	BrokerAddr         string
	RedisURL           string
	CertFile           string
	KeyFile            string
}

// Server is our top level type, contains all other components, is responsible
// for starting and stopping them in the correct order.
type Server struct {
	srv      *http.Server
	encoder  encoder.Encoder
	db       *postgres.DB
	mqtt     mqtt.Client
	logger   kitlog.Logger
	rd       *redis.Redis
	certFile string
	keyFile  string
}

// PulseHandler is the simplest possible handler function - used to expose an
// endpoint which a load balancer can ping to verify that a node is running and
// accepting connections.
func PulseHandler(db *postgres.DB, rd *redis.Redis) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		err := db.Ping()
		if err != nil {
			http.Error(w, "failed to connect to DB", http.StatusInternalServerError)
			return
		}
		err = rd.Ping()
		if err != nil {
			http.Error(w, "failed to connect to redis", http.StatusInternalServerError)
			return
		}
		fmt.Fprintf(w, "ok")
	})
}

// NewServer returns a new simple HTTP server. Is also responsible for
// constructing all components, and injecting them into the right place. This
// perhaps belongs elsewhere, but leaving here for now.
func NewServer(config *Config, logger kitlog.Logger) *Server {
	db := postgres.NewDB(&postgres.Config{
		ConnStr:            config.ConnStr,
		EncryptionPassword: config.EncryptionPassword,
		HashidSalt:         config.HashidSalt,
		HashidMinLength:    config.HashidMinLength,
	}, logger)

	ds := datastore.NewDatastoreProtobufClient(
		config.DatastoreAddr,
		&http.Client{
			Timeout: time.Second * 10,
		},
	)

	rd := redis.NewRedis(config.RedisURL, config.Verbose, redis.NewClock(), logger)

	processor := pipeline.NewProcessor(ds, rd, config.Verbose, logger)

	mqttClient := mqtt.NewClient(logger, config.Verbose)

	enc := rpc.NewEncoder(&rpc.Config{
		DB:         db,
		MQTTClient: mqttClient,
		Processor:  processor,
		Verbose:    config.Verbose,
		BrokerAddr: config.BrokerAddr,
	}, logger)

	hooks := twrpprom.NewServerHooks(nil)

	buildInfo.WithLabelValues(version.BinaryName, version.Version, version.BuildDate)

	logger = kitlog.With(logger, "module", "server")
	logger.Log("msg", "creating server", "datastore", config.DatastoreAddr, "hashid", config.HashidMinLength)

	twirpHandler := encoder.NewEncoderServer(enc, hooks)

	// multiplex twirp handler into a mux with our other handlers
	mux := goji.NewMux()

	mux.Handle(pat.Post(encoder.EncoderPathPrefix+"*"), twirpHandler)
	mux.Handle(pat.Get("/pulse"), PulseHandler(db, rd))
	mux.Handle(pat.Get("/metrics"), promhttp.Handler())

	mux.Use(middleware.RequestIDMiddleware)

	metricsMiddleware := middleware.MetricsMiddleware("decode", "encoder")
	mux.Use(metricsMiddleware)

	// create our http.Server instance
	srv := &http.Server{
		Addr:    config.ListenAddr,
		Handler: mux,
	}

	// return the instantiated server
	return &Server{
		srv:      srv,
		encoder:  enc,
		db:       db,
		mqtt:     mqttClient,
		logger:   kitlog.With(logger, "module", "server"),
		rd:       rd,
		certFile: config.CertFile,
		keyFile:  config.KeyFile,
	}
}

// Start starts the server running. This is responsible for starting components
// in the correct order, and in addition we attempt to run all up migrations as
// we start.
//
// We also create a channel listening for interrupt signals before gracefully
// shutting down.
func (s *Server) Start() error {
	// start the postgres connection pool
	err := s.db.Start()
	if err != nil {
		return errors.Wrap(err, "failed to start db")
	}

	// migrate up the database
	err = s.db.MigrateUp()
	if err != nil {
		return errors.Wrap(err, "failed to migrate the database")
	}

	err = s.rd.Start()
	if err != nil {
		return errors.Wrap(err, "failed to connect to redis")
	}

	// start the encoder RPC component - this creates all mqtt subscriptions
	err = s.encoder.(system.Startable).Start()
	if err != nil {
		return errors.Wrap(err, "failed to start encoder")
	}

	// add signal handling stuff to shutdown gracefully
	stopChan := make(chan os.Signal)
	signal.Notify(stopChan, os.Interrupt)

	go func() {
		s.logger.Log("listenAddr", s.srv.Addr, "msg", "starting server", "pathPrefix", encoder.EncoderPathPrefix, "tlsEnabled", isTLSEnabled(s.certFile, s.keyFile))

		if isTLSEnabled(s.certFile, s.keyFile) {
			if err := s.srv.ListenAndServeTLS(s.certFile, s.keyFile); err != nil {
				s.logger.Log("err", err)
				os.Exit(1)
			}
		} else {
			if err := s.srv.ListenAndServe(); err != nil {
				s.logger.Log("err", err)
				os.Exit(1)
			}
		}
	}()

	<-stopChan
	return s.Stop()
}

// Stop the server and all child components
func (s *Server) Stop() error {
	s.logger.Log("msg", "stopping")
	ctx, cancelFn := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancelFn()

	err := s.encoder.(system.Stoppable).Stop()
	if err != nil {
		return err
	}

	err = s.mqtt.(system.Stoppable).Stop()
	if err != nil {
		return err
	}

	err = s.rd.Stop()
	if err != nil {
		return err
	}

	err = s.db.Stop()
	if err != nil {
		return err
	}

	return s.srv.Shutdown(ctx)
}

// isTLSEnabled returns true if we have passed in paths for both cert and key
// files
func isTLSEnabled(certFile, keyFile string) bool {
	return certFile != "" && keyFile != ""
}
