package server

import (
	"context"
	"fmt"
	"log"
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
	registry "github.com/thingful/retryable-registry-prometheus"
	datastore "github.com/thingful/twirp-datastore-go"
	encoder "github.com/thingful/twirp-encoder-go"
	goji "goji.io"
	"goji.io/pat"
	"golang.org/x/crypto/acme/autocert"

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
	registry.MustRegister(buildInfo)
	registry.MustRegister(mqtt.MessageCounter)
	registry.MustRegister(pipeline.DatastoreErrorCounter)
	registry.MustRegister(pipeline.ZenroomErrorCounter)
	registry.MustRegister(pipeline.DatastoreWriteHistogram)
	registry.MustRegister(pipeline.ProcessHistogram)
	registry.MustRegister(pipeline.ZenroomHistogram)
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
	BrokerUsername     string
	RedisURL           string
	Domains            []string
}

// Server is our top level type, contains all other components, is responsible
// for starting and stopping them in the correct order.
type Server struct {
	srv     *http.Server
	encoder encoder.Encoder
	db      *postgres.DB
	mqtt    mqtt.Client
	logger  kitlog.Logger
	rd      *redis.Redis
	domains []string
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
		DB:             db,
		MQTTClient:     mqttClient,
		Processor:      processor,
		Verbose:        config.Verbose,
		BrokerAddr:     config.BrokerAddr,
		BrokerUsername: config.BrokerUsername,
	}, logger)

	hooks := twrpprom.NewServerHooks(registry.DefaultRegisterer)

	buildInfo.WithLabelValues(version.BinaryName, version.Version, version.BuildDate)

	logger = kitlog.With(logger, "module", "server")
	logger.Log(
		"msg", "creating server",
		"datastore", config.DatastoreAddr,
		"hashidLength", config.HashidMinLength,
		"mqttBroker", config.BrokerAddr,
		"listenAddr", config.ListenAddr,
		"mqttUsername", config.BrokerAddr,
	)

	twirpHandler := encoder.NewEncoderServer(enc, hooks)

	// multiplex twirp handler into a mux with our other handlers
	mux := goji.NewMux()

	mux.Handle(pat.Post(encoder.EncoderPathPrefix+"*"), twirpHandler)
	mux.Handle(pat.Get("/pulse"), PulseHandler(db, rd))
	mux.Handle(pat.Get("/metrics"), promhttp.Handler())

	mux.Use(middleware.RequestIDMiddleware)

	metricsMiddleware := middleware.MetricsMiddleware("decode", "encoder", registry.DefaultRegisterer)
	mux.Use(metricsMiddleware)

	// create our http.Server instance
	srv := &http.Server{
		Addr:    config.ListenAddr,
		Handler: mux,
	}

	// return the instantiated server
	return &Server{
		srv:     srv,
		encoder: enc,
		db:      db,
		mqtt:    mqttClient,
		logger:  kitlog.With(logger, "module", "server"),
		rd:      rd,
		domains: config.Domains,
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
		s.logger.Log(
			"listenAddr", s.srv.Addr,
			"msg", "starting server",
			"pathPrefix", encoder.EncoderPathPrefix,
			"tlsEnabled", isTLSEnabled(s.domains),
		)

		if isTLSEnabled(s.domains) {
			m := &autocert.Manager{
				Cache:      s.db,
				Prompt:     autocert.AcceptTOS,
				HostPolicy: autocert.HostWhitelist(s.domains...),
			}

			s.srv.TLSConfig = m.TLSConfig()

			if err := s.srv.ListenAndServeTLS("", ""); err != nil && err != http.ErrServerClosed {
				log.Fatalf("ListenAndServeTLS(): %s", err)
			}
		} else {
			if err := s.srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
				log.Fatalf("ListenAndServe(): %s", err)
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
func isTLSEnabled(domains []string) bool {
	return len(domains) > 0
}
