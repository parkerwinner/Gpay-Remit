package middleware

import (
	"net/http"
	"runtime/debug"

	"github.com/Gpay-Remit/gpay-remit/errors"
	"github.com/Gpay-Remit/gpay-remit/logger"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/sirupsen/logrus"
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

// ErrorResponse is the standardized JSON error response. Every error body the
// API returns has this shape: a machine-readable code, a human-readable
// message, optional details, and the request ID for log correlation.
type ErrorResponse struct {
	Error struct {
		Code      errors.ErrorCode `json:"code"`
		Message   string           `json:"message"`
		Details   interface{}      `json:"details,omitempty"`
		RequestID string           `json:"request_id,omitempty"`
	} `json:"error"`
}

// newErrorResponse builds the standard error body for the request in c.
func newErrorResponse(c *gin.Context, code errors.ErrorCode, message string, details interface{}) ErrorResponse {
	resp := ErrorResponse{}
	resp.Error.Code = code
	resp.Error.Message = message
	resp.Error.Details = details
	resp.Error.RequestID = c.GetString("requestID")
	return resp
}

// AbortWithError writes the standard error response with the given status and
// stops the handler chain. Middleware uses it so the response is correct even
// when the global ErrorHandler is not in the chain. Inside handlers prefer
// c.Error(...) with an *errors.AppError, which ErrorHandler renders and logs.
func AbortWithError(c *gin.Context, status int, code errors.ErrorCode, message string, details interface{}) {
	c.AbortWithStatusJSON(status, newErrorResponse(c, code, message, details))
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

				logger.Log.WithFields(logrus.Fields{
					"panic":      err,
					"stack":      string(stack),
					"request_id": requestID,
					"user_id":    userID,
					"path":       c.Request.URL.Path,
				}).Error("Panic recovered")

				// Standard 500 response
				resp := newErrorResponse(c, errors.CodeInternal, "An internal server error occurred", nil)

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
				logger.Log.WithFields(logFields).Error(appErr.Message)
			} else {
				logger.Log.WithFields(logFields).Warn(appErr.Message)
			}

			// Prepare response - hide internal details for 500 errors
			message := appErr.Message
			if appErr.HTTPStatus >= 500 {
				message = "An internal server error occurred"
			}

			resp := newErrorResponse(c, appErr.Code, message, appErr.Details)

			// If response was already written, we can't change it
			if !c.Writer.Written() {
				c.JSON(appErr.HTTPStatus, resp)
			}
		}
	}
}
