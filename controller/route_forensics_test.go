package controller

import (
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAppendRouteForensicsClassifiesNoFirstByteHang(t *testing.T) {
	ctx := newRouteForensicsContext(t, "route-hang")
	start := time.Now().Add(-121 * time.Second)
	relayInfo := &relaycommon.RelayInfo{
		RequestId:         "route-hang",
		StartTime:         start,
		FirstResponseTime: start.Add(-time.Second),
		RetryIndex:        1,
	}

	other := map[string]interface{}{}
	appendRouteForensics(ctx, relayInfo, 502, 121, other)

	forensics := requireRouteForensics(t, other)
	assert.Equal(t, "no_first_byte_hang", forensics["classification"])
	assert.Equal(t, int64(-1000), forensics["first_response_ms"])
	assert.Equal(t, int64(121000), forensics["elapsed_ms"])
	assert.Equal(t, 1, forensics["retry_index"])
	assert.Equal(t, true, forensics["threshold_exceeded"])
	assert.Equal(t, "route-hang", forensics["request_id"])
}

func TestAppendRouteForensicsClassifiesUpstream502WithoutHang(t *testing.T) {
	ctx := newRouteForensicsContext(t, "route-502")
	ctx.Set(common.UpstreamRequestIdKey, "upstream-502")
	start := time.Now().Add(-3 * time.Second)
	relayInfo := &relaycommon.RelayInfo{
		RequestId:         "route-502",
		StartTime:         start,
		FirstResponseTime: start.Add(250 * time.Millisecond),
	}

	other := map[string]interface{}{}
	appendRouteForensics(ctx, relayInfo, 502, 3, other)

	forensics := requireRouteForensics(t, other)
	assert.Equal(t, "upstream_502", forensics["classification"])
	assert.Equal(t, int64(250), forensics["first_response_ms"])
	assert.Equal(t, false, forensics["threshold_exceeded"])
	assert.Equal(t, "upstream-502", forensics["upstream_request_id"])
}

func newRouteForensicsContext(t *testing.T, requestID string) *gin.Context {
	t.Helper()
	ctx, _ := gin.CreateTestContext(nil)
	ctx.Set(common.RequestIdKey, requestID)
	return ctx
}

func requireRouteForensics(t *testing.T, other map[string]interface{}) map[string]interface{} {
	t.Helper()
	forensics, ok := other["route_forensics"].(map[string]interface{})
	require.True(t, ok)
	return forensics
}
