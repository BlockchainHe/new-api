package openai

import (
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func withResponsesStreamTestMode(t *testing.T) {
	t.Helper()
	oldMode := gin.Mode()
	gin.SetMode(gin.TestMode)
	t.Cleanup(func() { gin.SetMode(oldMode) })

	oldTimeout := constant.StreamingTimeout
	constant.StreamingTimeout = 30
	t.Cleanup(func() { constant.StreamingTimeout = oldTimeout })
}

func countDoneMarkers(body string) int {
	return strings.Count(body, "data: [DONE]")
}

func hasTrailingJSONError(body string) bool {
	// Reject a standalone JSON error object appended after SSE frames.
	for _, line := range strings.Split(body, "\n") {
		trim := strings.TrimSpace(line)
		if trim == "" || strings.HasPrefix(trim, "data:") || strings.HasPrefix(trim, "event:") || strings.HasPrefix(trim, ":") {
			continue
		}
		if strings.HasPrefix(trim, "{") && strings.Contains(trim, `"error"`) {
			return true
		}
	}
	return false
}

func TestOaiResponsesStreamHandlerCompletedDoneOnce(t *testing.T) {
	withResponsesStreamTestMode(t)

	body := strings.Join([]string{
		`data: {"type":"response.created","response":{"id":"resp_1","model":"gpt-test","created_at":1710000000}}`,
		`data: {"type":"response.output_text.delta","delta":"hello"}`,
		`data: {"type":"response.completed","response":{"id":"resp_1","model":"gpt-test","status":"completed","usage":{"input_tokens":2,"output_tokens":3,"total_tokens":5}}}`,
		`data: [DONE]`,
		``,
	}, "\n")

	c, recorder, resp, info := newResponsesChatTestContext(t, body, true)
	usage, err := OaiResponsesStreamHandler(c, info, resp)

	require.Nil(t, err)
	require.Equal(t, 5, usage.TotalTokens)
	out := recorder.Body.String()
	require.Contains(t, out, `event: response.created`)
	require.Contains(t, out, `event: response.completed`)
	require.Equal(t, 1, strings.Count(out, `"type":"response.completed"`))
	require.Equal(t, 1, countDoneMarkers(out))
	require.False(t, hasTrailingJSONError(out))
	term, _ := c.Get(responsesStreamTerminalKey)
	require.Equal(t, "response.completed", term)
}

func TestOaiResponsesStreamHandlerIncompleteIsLegalTerminal(t *testing.T) {
	withResponsesStreamTestMode(t)

	body := strings.Join([]string{
		`data: {"type":"response.created","response":{"id":"resp_1","model":"gpt-test","created_at":1710000000}}`,
		`data: {"type":"response.output_text.delta","delta":"partial"}`,
		`data: {"type":"response.incomplete","response":{"id":"resp_1","model":"gpt-test","status":"incomplete","usage":{"input_tokens":2,"output_tokens":1,"total_tokens":3}}}`,
		``,
	}, "\n")

	c, recorder, resp, info := newResponsesChatTestContext(t, body, true)
	usage, err := OaiResponsesStreamHandler(c, info, resp)

	require.Nil(t, err)
	require.Equal(t, 3, usage.TotalTokens)
	out := recorder.Body.String()
	require.Contains(t, out, `response.incomplete`)
	require.NotContains(t, out, `"status":"completed"`)
	require.Equal(t, 1, countDoneMarkers(out))
	require.False(t, hasTrailingJSONError(out))
	term, _ := c.Get(responsesStreamTerminalKey)
	require.Equal(t, "response.incomplete", term)
	require.NotEqual(t, "response.completed", term)
}

func TestOaiResponsesStreamHandlerFailedAfterCreated(t *testing.T) {
	withResponsesStreamTestMode(t)

	body := strings.Join([]string{
		`data: {"type":"response.created","response":{"id":"resp_1","model":"gpt-test","created_at":1710000000}}`,
		`data: {"type":"response.failed","response":{"id":"resp_1","status":"failed","error":{"message":"upstream boom"}}}`,
		``,
	}, "\n")

	c, recorder, resp, info := newResponsesChatTestContext(t, body, true)
	usage, err := OaiResponsesStreamHandler(c, info, resp)

	require.Nil(t, usage)
	require.NotNil(t, err)
	require.True(t, types.IsSkipRetryError(err))
	out := recorder.Body.String()
	require.Contains(t, out, `response.created`)
	require.Contains(t, out, `response.failed`)
	require.Equal(t, 0, countDoneMarkers(out))
	require.False(t, hasTrailingJSONError(out))
	require.True(t, c.Writer.Written())
}

func TestOaiResponsesStreamHandlerErrorAfterCreated(t *testing.T) {
	withResponsesStreamTestMode(t)

	body := strings.Join([]string{
		`data: {"type":"response.created","response":{"id":"resp_1","model":"gpt-test","created_at":1710000000}}`,
		`data: {"type":"response.error","response":{"id":"resp_1","status":"failed"}}`,
		``,
	}, "\n")

	c, recorder, resp, info := newResponsesChatTestContext(t, body, true)
	usage, err := OaiResponsesStreamHandler(c, info, resp)

	require.Nil(t, usage)
	require.NotNil(t, err)
	require.True(t, types.IsSkipRetryError(err))
	out := recorder.Body.String()
	require.Contains(t, out, `response.error`)
	require.Equal(t, 0, countDoneMarkers(out))
	require.False(t, hasTrailingJSONError(out))
}

func TestOaiResponsesStreamHandlerEOFAfterDelta(t *testing.T) {
	withResponsesStreamTestMode(t)

	body := strings.Join([]string{
		`data: {"type":"response.created","response":{"id":"resp_1","model":"gpt-test","created_at":1710000000}}`,
		`data: {"type":"response.output_text.delta","delta":"hello"}`,
		``,
	}, "\n")

	c, recorder, resp, info := newResponsesChatTestContext(t, body, true)
	usage, err := OaiResponsesStreamHandler(c, info, resp)

	require.Nil(t, usage)
	require.NotNil(t, err)
	require.Equal(t, types.ErrorCodeEmptyResponse, err.GetErrorCode())
	require.True(t, types.IsSkipRetryError(err))
	out := recorder.Body.String()
	require.Contains(t, out, `response.output_text.delta`)
	require.Equal(t, 0, countDoneMarkers(out))
	require.False(t, hasTrailingJSONError(out))
}

func TestOaiResponsesStreamHandlerEmptyStreamNoDone(t *testing.T) {
	withResponsesStreamTestMode(t)

	body := "\n"
	c, recorder, resp, info := newResponsesChatTestContext(t, body, true)
	usage, err := OaiResponsesStreamHandler(c, info, resp)

	require.Nil(t, usage)
	require.NotNil(t, err)
	require.Equal(t, types.ErrorCodeEmptyResponse, err.GetErrorCode())
	// Before any body write, skipRetry must stay false so controller may retry.
	if !c.Writer.Written() {
		require.False(t, types.IsSkipRetryError(err))
	}
	require.Equal(t, 0, countDoneMarkers(recorder.Body.String()))
	require.False(t, hasTrailingJSONError(recorder.Body.String()))
}

func TestOaiResponsesStreamHandlerUpstreamDoneOnlyOnce(t *testing.T) {
	withResponsesStreamTestMode(t)

	// Upstream may emit [DONE]; scanner consumes it and handler emits exactly one.
	body := strings.Join([]string{
		`data: {"type":"response.created","response":{"id":"resp_1","model":"gpt-test","created_at":1710000000}}`,
		`data: {"type":"response.completed","response":{"id":"resp_1","status":"completed","usage":{"input_tokens":1,"output_tokens":1,"total_tokens":2}}}`,
		`data: [DONE]`,
		``,
	}, "\n")

	c, recorder, resp, info := newResponsesChatTestContext(t, body, true)
	usage, err := OaiResponsesStreamHandler(c, info, resp)

	require.Nil(t, err)
	require.Equal(t, 2, usage.TotalTokens)
	require.Equal(t, 1, countDoneMarkers(recorder.Body.String()))
}
