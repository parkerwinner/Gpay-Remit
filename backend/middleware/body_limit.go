package middleware

import (
	"fmt"
	"net/http"

	"github.com/Gpay-Remit/gpay-remit/errors"
	"github.com/gin-gonic/gin"
)

const (
	// DefaultMaxBodyBytes is the request body limit applied to every route
	// unless a route-specific override is configured (1 MiB).
	DefaultMaxBodyBytes int64 = 1 << 20

	// UploadMaxBodyBytes is a suggested limit for file-upload routes (10 MiB).
	UploadMaxBodyBytes int64 = 10 << 20
)

// BodyLimit caps the size of request bodies.
//
// defaultMax is the limit for routes without an entry in routeLimits. routeLimits
// is keyed by Gin route pattern (for example "/api/v1/uploads") and lets a
// specific route accept a larger or smaller body. A limit of zero or less
// disables the check for that route.
//
// Requests whose declared Content-Length exceeds the limit are rejected with
// 413 Payload Too Large before the handler runs. For requests without a
// declared length (chunked transfer), the body is wrapped with
// http.MaxBytesReader so reads stop at the limit; the handler then sees a read
// error.
func BodyLimit(defaultMax int64, routeLimits map[string]int64) gin.HandlerFunc {
	return func(c *gin.Context) {
		limit := defaultMax
		if routeMax, ok := routeLimits[c.FullPath()]; ok {
			limit = routeMax
		}

		if limit <= 0 || c.Request.Body == nil {
			c.Next()
			return
		}

		if c.Request.ContentLength > limit {
			AbortWithError(c, http.StatusRequestEntityTooLarge, errors.CodePayloadTooLarge,
				fmt.Sprintf("request body exceeds maximum size of %d bytes", limit), nil)
			return
		}

		c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, limit)
		c.Next()
	}
}
