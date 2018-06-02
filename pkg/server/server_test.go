package server_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

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
