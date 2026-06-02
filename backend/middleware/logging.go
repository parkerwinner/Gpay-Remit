package middleware

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"
	"github.com/yourusername/gpay-remit/logger"
)

// maxBodyBytes is the maximum number of request/response body bytes captured
// for logging. Bodies larger than this are truncated to avoid memory pressure.
const maxBodyBytes = 4 * 1024 // 4 KB

// responseBodyWriter wraps gin.ResponseWriter so we can tee the response body
// into an in-memory buffer for post-handler logging.
type responseBodyWriter struct {
	gin.ResponseWriter
	buf *bytes.Buffer
}

func (w *responseBodyWriter) Write(b []byte) (int, error) {
	if w.buf.Len() < maxBodyBytes {
		remaining := maxBodyBytes - w.buf.Len()
		if len(b) > remaining {
			w.buf.Write(b[:remaining])
		} else {
			w.buf.Write(b)
		}
	}
	return w.ResponseWriter.Write(b)
}

// RequestLogger returns a Gin middleware that logs every HTTP request with
// structured fields including:
//   - method, path, query, status, duration, client IP
//   - correlation_id and request_id for distributed tracing
//   - sanitized request body (JSON only, up to maxBodyBytes)
//   - sanitized response body (JSON only, up to maxBodyBytes)
//
// Sensitive fields (passwords, tokens, etc.) are redacted automatically.
// Body capture is skipped for non-JSON content types to avoid overhead.
func RequestLogger() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		path := c.Request.URL.Path
		query := c.Request.URL.RawQuery

		// --- Correlation ID ---
		// We re-use the requestID already set by RequestIDMiddleware and treat
		// the client-supplied X-Correlation-ID (or a generated fallback) as the
		// cross-service correlation identifier.
		correlationID := c.GetHeader("X-Correlation-ID")
		if correlationID == "" {
			// Fall back to the request ID so every log line is still traceable.
			correlationID = c.GetString("requestID")
		}
		c.Set("correlationID", correlationID)
		c.Header("X-Correlation-ID", correlationID)

		// --- Capture request body ---
		var reqBody interface{}
		if shouldCaptureBody(c.Request) {
			reqBody = readAndRestoreBody(c)
		}

		// --- Wrap response writer to capture response body ---
		resBuf := &bytes.Buffer{}
		rbw := &responseBodyWriter{ResponseWriter: c.Writer, buf: resBuf}
		c.Writer = rbw

		// --- Execute handlers ---
		c.Next()

		duration := time.Since(start)
		status := c.Writer.Status()

		// --- Decode response body ---
		var resBody interface{}
		if isJSONContentType(c.Writer.Header().Get("Content-Type")) {
			resBody = decodeJSON(resBuf.Bytes())
		}

		// --- Build log entry ---
		fields := logrus.Fields{
			"method":         c.Request.Method,
			"path":           path,
			"status":         status,
			"duration_ms":    duration.Milliseconds(),
			"ip":             c.ClientIP(),
			"request_id":     c.GetString("requestID"),
			"correlation_id": correlationID,
		}

		if query != "" {
			fields["query"] = query
		}

		userID, exists := c.Get("userID")
		if exists {
			fields["user_id"] = userID
		}

		if reqBody != nil {
			fields["request_body"] = logger.SanitizeBody(reqBody)
		}

		if resBody != nil {
			fields["response_body"] = logger.SanitizeBody(resBody)
		}

		entry := logger.Log.WithFields(fields)

		switch {
		case status >= http.StatusInternalServerError:
			entry.Error("HTTP request")
		case status >= http.StatusBadRequest:
			entry.Warn("HTTP request")
		default:
			entry.Info("HTTP request")
		}
	}
}

// shouldCaptureBody returns true when the request body is worth capturing:
// the method can carry a body and the content type is JSON.
func shouldCaptureBody(r *http.Request) bool {
	if r.Body == nil || r.ContentLength == 0 {
		return false
	}
	switch r.Method {
	case http.MethodPost, http.MethodPut, http.MethodPatch:
		return isJSONContentType(r.Header.Get("Content-Type"))
	}
	return false
}

// isJSONContentType returns true for application/json (with or without charset).
func isJSONContentType(ct string) bool {
	return strings.HasPrefix(ct, "application/json")
}

// readAndRestoreBody drains up to maxBodyBytes from the request body, puts the
// full original body back so downstream handlers can still read it, and returns
// the captured bytes decoded as a JSON value.
func readAndRestoreBody(c *gin.Context) interface{} {
	limitedReader := io.LimitReader(c.Request.Body, int64(maxBodyBytes)+1)
	captured, err := io.ReadAll(limitedReader)
	if err != nil {
		return nil
	}

	// Restore the body so the actual handler can still parse it.
	c.Request.Body = io.NopCloser(
		io.MultiReader(bytes.NewReader(captured), c.Request.Body),
	)

	if len(captured) == 0 {
		return nil
	}
	return decodeJSON(captured)
}

// decodeJSON attempts to unmarshal b into a map. Returns nil on failure so
// callers don't log raw bytes.
func decodeJSON(b []byte) interface{} {
	if len(b) == 0 {
		return nil
	}
	var v map[string]interface{}
	if err := json.Unmarshal(b, &v); err != nil {
		return nil
	}
	return v
}
