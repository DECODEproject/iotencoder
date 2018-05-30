package server_test

import (
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	kitlog "github.com/go-kit/kit/log"
	"github.com/stretchr/testify/assert"

	"github.com/thingful/iotencoder/pkg/server"
)

func TestPulseHandler(t *testing.T) {
	req, err := http.NewRequest(http.MethodGet, "/pulse", nil)
	assert.Nil(t, err)

	rr := httptest.NewRecorder()
	handler := http.HandlerFunc(server.PulseHandler)

	handler.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)
}

func TestStartStop(t *testing.T) {
	logger := kitlog.NewNopLogger()
	s := server.NewServer(
		&server.Config{
			ListenAddr:         "127.0.0.1:0",
			ConnStr:            os.Getenv("IOTENCODER_DATABASE_URL"),
			EncryptionPassword: "password",
			HashidSalt:         "salt",
			HashidMinLength:    8,
			DatastoreAddr:      "127.0.0.1:9999",
		},
		logger,
	)

	go func() {
		s.Start()
	}()

	time.Sleep(time.Second * 1)

	err := s.Stop()
	if err != nil {
		t.Errorf("Unexpected error on Stop: %v", err)
	}
}
