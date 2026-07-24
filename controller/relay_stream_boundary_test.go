package controller

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestResponseBodyStartedAndShouldRetry(t *testing.T) {
	oldMode := gin.Mode()
	gin.SetMode(gin.TestMode)
	t.Cleanup(func() { gin.SetMode(oldMode) })

	// Before write: body not started, 502 may retry (status-code policy).
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	require.False(t, responseBodyStarted(c))

	apiErr := types.NewOpenAIError(
		http.ErrHandlerTimeout,
		types.ErrorCodeBadResponse,
		http.StatusBadGateway,
	)
	// Even with retries remaining, only status-code policy applies; Written=false.
	// We only assert the Written guard: after write, retry is always false.
	require.False(t, responseBodyStarted(c))

	// After write: must never retry and must report body started.
	_, _ = c.Writer.Write([]byte("data: {\"type\":\"response.created\"}\n\n"))
	require.True(t, responseBodyStarted(c))
	require.False(t, shouldRetry(c, apiErr, 3))
}

func TestShouldRetryNilError(t *testing.T) {
	oldMode := gin.Mode()
	gin.SetMode(gin.TestMode)
	t.Cleanup(func() { gin.SetMode(oldMode) })

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	require.False(t, shouldRetry(c, nil, 3))
}

func TestShouldRetrySkipRetryFlag(t *testing.T) {
	oldMode := gin.Mode()
	gin.SetMode(gin.TestMode)
	t.Cleanup(func() { gin.SetMode(oldMode) })

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)

	apiErr := types.NewOpenAIError(
		http.ErrHandlerTimeout,
		types.ErrorCodeBadResponse,
		http.StatusBadGateway,
		types.ErrOptionWithSkipRetry(),
	)
	require.False(t, shouldRetry(c, apiErr, 3))
}
