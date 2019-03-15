package server_test

import (
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/DECODEproject/iotencoder/pkg/redis"

	kitlog "github.com/go-kit/kit/log"
	"github.com/stretchr/testify/assert"

	"github.com/DECODEproject/iotencoder/pkg/postgres"
	"github.com/DECODEproject/iotencoder/pkg/server"
)

func TestPulseHandler(t *testing.T) {
	req, err := http.NewRequest(http.MethodGet, "/pulse", nil)
	assert.Nil(t, err)

	connStr := os.Getenv("IOTENCODER_DATABASE_URL")
	redisStr := os.Getenv("IOTENCODER_REDIS_URL")

	logger := kitlog.NewNopLogger()

	db := postgres.NewDB(
		&postgres.Config{
			ConnStr:            connStr,
			EncryptionPassword: "password",
		},
		logger,
	)

	err = db.Start()
	assert.Nil(t, err)

	defer db.Stop()

	rd := redis.NewRedis(
		redisStr,
		false,
		redis.NewClock(),
		logger,
	)

	err = rd.Start()
	assert.Nil(t, err)

	rr := httptest.NewRecorder()
	handler := server.PulseHandler(db, rd)

	handler.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)
}
