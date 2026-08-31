package handlers

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/yourusername/gpay-remit/errors"
)

// errorHandlerForTests is a lightweight Gin middleware that mirrors the
// production ErrorHandler's behaviour: it converts c.Errors entries into
// proper JSON responses with the correct HTTP status code.
// Using this in tests avoids a dependency on the middleware package and
// keeps the handlers package self-contained.
func errorHandlerForTests() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Next()

		if len(c.Errors) == 0 {
			return
		}

		ginErr := c.Errors.Last()
		if c.Writer.Written() {
			return
		}

		var appErr *errors.AppError
		var ok bool
		if appErr, ok = ginErr.Err.(*errors.AppError); !ok {
			c.JSON(http.StatusInternalServerError, gin.H{"error": ginErr.Error()})
			return
		}

		c.JSON(appErr.HTTPStatus, gin.H{"error": appErr.Message})
	}
}
