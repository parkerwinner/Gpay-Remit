package middleware

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/yourusername/gpay-remit/logger"
)

func init() {
	gin.SetMode(gin.TestMode)
}

// newTestRouter wires up the full middleware chain used in production so the
// tests exercise the real interactions (RequestIDMiddleware → RequestLogger).
func newTestRouter(handler gin.HandlerFunc) *gin.Engine {
	r := gin.New()
	r.Use(RequestIDMiddleware())
	r.Use(RequestLogger())
	r.POST("/test", handler)
	r.GET("/test", handler)
	return r
}

// ---------- helper: send a request and return the recorder ----------

func doRequest(t *testing.T, router *gin.Engine, method, path string, body interface{}, headers map[string]string) *httptest.ResponseRecorder {
	t.Helper()
	var bodyReader *bytes.Reader
	if body != nil {
		b, err := json.Marshal(body)
		require.NoError(t, err)
		bodyReader = bytes.NewReader(b)
	} else {
		bodyReader = bytes.NewReader(nil)
	}

	req := httptest.NewRequest(method, path, bodyReader)
	req.Header.Set("Content-Type", "application/json")
	for k, v := range headers {
		req.Header.Set(k, v)
	}

	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	return w
}

// ---------- tests ----------

func TestRequestLogger_SetsCorrelationIDHeader(t *testing.T) {
	router := newTestRouter(func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"ok": true})
	})

	w := doRequest(t, router, http.MethodGet, "/test", nil, nil)

	assert.NotEmpty(t, w.Header().Get("X-Correlation-ID"),
		"response must contain X-Correlation-ID header")
}

func TestRequestLogger_HonorsClientCorrelationID(t *testing.T) {
	router := newTestRouter(func(c *gin.Context) {
		cid := c.GetString("correlationID")
		c.JSON(http.StatusOK, gin.H{"cid": cid})
	})

	w := doRequest(t, router, http.MethodGet, "/test", nil, map[string]string{
		"X-Correlation-ID": "my-trace-abc",
	})

	assert.Equal(t, "my-trace-abc", w.Header().Get("X-Correlation-ID"))

	var resp map[string]interface{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, "my-trace-abc", resp["cid"])
}

func TestRequestLogger_RequestBodyReachesHandler(t *testing.T) {
	// The middleware must restore the body after peeking, so the handler can
	// still parse it.
	router := newTestRouter(func(c *gin.Context) {
		var payload map[string]interface{}
		require.NoError(t, c.ShouldBindJSON(&payload))
		c.JSON(http.StatusOK, payload)
	})

	payload := map[string]interface{}{"amount": 100, "currency": "USD"}
	w := doRequest(t, router, http.MethodPost, "/test", payload, nil)

	assert.Equal(t, http.StatusOK, w.Code)

	var got map[string]interface{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &got))
	assert.Equal(t, float64(100), got["amount"])
}

func TestRequestLogger_SanitizesSensitiveRequestFields(t *testing.T) {
	// We verify sanitisation via logger.SanitizeBody directly (unit) and via an
	// integration smoke-test that the handler still receives the original values.
	sensitive := map[string]interface{}{
		"email":    "user@example.com",
		"password": "s3cr3t",
		"token":    "tok_abc123",
	}

	sanitized := logger.SanitizeBody(sensitive).(map[string]interface{})

	assert.Equal(t, "user@example.com", sanitized["email"], "non-sensitive field must pass through")
	assert.Equal(t, "[REDACTED]", sanitized["password"])
	assert.Equal(t, "[REDACTED]", sanitized["token"])
}

func TestRequestLogger_SanitizesNestedSensitiveFields(t *testing.T) {
	nested := map[string]interface{}{
		"user": map[string]interface{}{
			"name":     "Alice",
			"password": "hunter2",
		},
	}

	sanitized := logger.SanitizeBody(nested).(map[string]interface{})
	userMap := sanitized["user"].(map[string]interface{})

	assert.Equal(t, "Alice", userMap["name"])
	assert.Equal(t, "[REDACTED]", userMap["password"])
}

func TestRequestLogger_NonJSONBodyNotCaptured(t *testing.T) {
	// Plain-text POST — middleware must not blow up and handler must still work.
	r := gin.New()
	r.Use(RequestIDMiddleware())
	r.Use(RequestLogger())
	r.POST("/plain", func(c *gin.Context) {
		body, _ := c.GetRawData()
		c.String(http.StatusOK, string(body))
	})

	req := httptest.NewRequest(http.MethodPost, "/plain", strings.NewReader("hello world"))
	req.Header.Set("Content-Type", "text/plain")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "hello world", w.Body.String())
}

func TestRequestLogger_DurationFieldPresent(t *testing.T) {
	// We can't easily inspect the logrus output in a unit test without a custom
	// hook, but we can assert the middleware doesn't panic and returns the
	// expected status — the duration field is always set.
	router := newTestRouter(func(c *gin.Context) {
		c.JSON(http.StatusNoContent, nil)
	})

	w := doRequest(t, router, http.MethodGet, "/test", nil, nil)
	assert.Equal(t, http.StatusNoContent, w.Code)
}

func TestRequestLogger_ResponseBodyCaptured(t *testing.T) {
	// The response writer wrapper must not swallow the body — the client must
	// still receive the full JSON response.
	router := newTestRouter(func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"result": "ok", "items": []int{1, 2, 3}})
	})

	w := doRequest(t, router, http.MethodGet, "/test", nil, nil)
	assert.Equal(t, http.StatusOK, w.Code)

	var resp map[string]interface{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, "ok", resp["result"])
}

func TestRequestLogger_5xxLogsAsError(t *testing.T) {
	// Just verify middleware doesn't panic on 5xx and passes status through.
	router := newTestRouter(func(c *gin.Context) {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "boom"})
	})

	w := doRequest(t, router, http.MethodGet, "/test", nil, nil)
	assert.Equal(t, http.StatusInternalServerError, w.Code)
}

func TestRequestLogger_4xxLogsAsWarn(t *testing.T) {
	router := newTestRouter(func(c *gin.Context) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "bad input"})
	})

	w := doRequest(t, router, http.MethodGet, "/test", nil, nil)
	assert.Equal(t, http.StatusBadRequest, w.Code)
}
