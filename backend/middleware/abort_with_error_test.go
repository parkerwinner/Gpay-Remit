package middleware

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Gpay-Remit/gpay-remit/errors"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAbortWithErrorWritesStandardEnvelopeWithRequestID(t *testing.T) {
	gin.SetMode(gin.TestMode)

	router := gin.New()
	router.Use(RequestIDMiddleware())
	router.GET("/abort", func(c *gin.Context) {
		AbortWithError(c, http.StatusTooManyRequests, errors.CodeRateLimited, "slow down", gin.H{"retry_after": 30})
	})

	req := httptest.NewRequest(http.MethodGet, "/abort", nil)
	req.Header.Set("X-Request-ID", "req-abc")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusTooManyRequests, w.Code)
	assert.Equal(t, "req-abc", w.Header().Get("X-Request-ID"))

	var body ErrorResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	assert.Equal(t, errors.CodeRateLimited, body.Error.Code)
	assert.Equal(t, "slow down", body.Error.Message)
	assert.Equal(t, "req-abc", body.Error.RequestID)
	assert.NotNil(t, body.Error.Details)
}

func TestAbortWithErrorOmitsRequestIDWhenNoRequestIDMiddleware(t *testing.T) {
	gin.SetMode(gin.TestMode)

	router := gin.New()
	router.GET("/abort", func(c *gin.Context) {
		AbortWithError(c, http.StatusNotFound, errors.CodeNotFound, "missing", nil)
	})

	w := httptest.NewRecorder()
	router.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/abort", nil))

	assert.Equal(t, http.StatusNotFound, w.Code)
	assert.NotContains(t, w.Body.String(), "request_id")
	assert.NotContains(t, w.Body.String(), "details")
}
