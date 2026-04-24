package middleware

import (
	"fmt"
	"net/http"
	"runtime/debug"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/sirupsen/logrus"
	"github.com/yourusername/gpay-remit/errors"
)

// RequestIDMiddleware adds a unique request ID to each request
func RequestIDMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		requestID := c.GetHeader("X-Request-ID")
		if requestID == "" {
			requestID = uuid.New().String()
		}
		c.Set("requestID", requestID)
		c.Header("X-Request-ID", requestID)
		c.Next()
	}
}

// ErrorResponse is the standardized JSON error response
type ErrorResponse struct {
	Error struct {
		Code    errors.ErrorCode `json:"code"`
		Message string           `json:"message"`
		Details interface{}      `json:"details,omitempty"`
	} `json:"error"`
}

// ErrorHandler handles panics and standardized error responses
func ErrorHandler() gin.HandlerFunc {
	return func(c *gin.Context) {
		defer func() {
			if err := recover(); err != nil {
				stack := debug.Stack()
				
				// Extract context info for logging
				requestID := c.GetString("requestID")
				userID, _ := c.Get("userID")

				logrus.WithFields(logrus.Fields{
					"panic":      err,
					"stack":      string(stack),
					"request_id": requestID,
					"user_id":    userID,
					"path":       c.Request.URL.Path,
				}).Error("Panic recovered")

				// Standard 500 response
				resp := ErrorResponse{}
				resp.Error.Code = errors.CodeInternal
				resp.Error.Message = "An internal server error occurred"
				
				c.AbortWithStatusJSON(http.StatusInternalServerError, resp)
			}
		}()

		c.Next()

		// If there are errors in the context, handle them
		if len(c.Errors) > 0 {
			// Get the last error
			ginErr := c.Errors.Last()
			err := ginErr.Err

			var appErr *errors.AppError
			var ok bool
			
			if appErr, ok = err.(*errors.AppError); !ok {
				// Wrap unknown errors as internal errors
				appErr = errors.NewInternalError("An unexpected error occurred", err)
			}

			// Extract context info for logging
			requestID := c.GetString("requestID")
			userID, _ := c.Get("userID")

			logFields := logrus.Fields{
				"code":       appErr.Code,
				"request_id": requestID,
				"user_id":    userID,
				"path":       c.Request.URL.Path,
				"method":     c.Request.Method,
			}
			if appErr.Err != nil {
				logFields["inner_error"] = appErr.Err.Error()
			}

			// Log based on severity
			if appErr.HTTPStatus >= 500 {
				logrus.WithFields(logFields).Error(appErr.Message)
			} else {
				logrus.WithFields(logFields).Warn(appErr.Message)
			}

			// Prepare response - hide internal details for 500 errors
			message := appErr.Message
			if appErr.HTTPStatus >= 500 {
				message = "An internal server error occurred"
			}

			resp := ErrorResponse{}
			resp.Error.Code = appErr.Code
			resp.Error.Message = message
			resp.Error.Details = appErr.Details

			// If response was already written, we can't change it
			if !c.Writer.Written() {
				c.JSON(appErr.HTTPStatus, resp)
			}
		}
	}
}
