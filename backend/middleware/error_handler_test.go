package middleware

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/yourusername/gpay-remit/errors"
)

func TestErrorHandler(t *testing.T) {
	gin.SetMode(gin.TestMode)

	t.Run("Handle panic", func(t *testing.T) {
		router := gin.New()
		router.Use(ErrorHandler())
		router.GET("/panic", func(c *gin.Context) {
			panic("something went wrong")
		})

		w := httptest.NewRecorder()
		req, _ := http.NewRequest("GET", "/panic", nil)
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusInternalServerError, w.Code)
		
		var resp ErrorResponse
		err := json.Unmarshal(w.Body.Bytes(), &resp)
		assert.NoError(t, err)
		assert.Equal(t, errors.CodeInternal, resp.Error.Code)
		assert.Equal(t, "An internal server error occurred", resp.Error.Message)
	})

	t.Run("Handle AppError", func(t *testing.T) {
		router := gin.New()
		router.Use(ErrorHandler())
		router.GET("/error", func(c *gin.Context) {
			c.Error(errors.NewValidationError("Invalid input", map[string]string{"field": "required"}))
		})

		w := httptest.NewRecorder()
		req, _ := http.NewRequest("GET", "/error", nil)
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code)
		
		var resp ErrorResponse
		err := json.Unmarshal(w.Body.Bytes(), &resp)
		assert.NoError(t, err)
		assert.Equal(t, errors.CodeValidation, resp.Error.Code)
		assert.Equal(t, "Invalid input", resp.Error.Message)
		assert.NotNil(t, resp.Error.Details)
	})

	t.Run("Handle generic error", func(t *testing.T) {
		router := gin.New()
		router.Use(ErrorHandler())
		router.GET("/generic", func(c *gin.Context) {
			c.Error(json.Unmarshal([]byte("{invalid"), &struct{}{}))
		})

		w := httptest.NewRecorder()
		req, _ := http.NewRequest("GET", "/generic", nil)
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusInternalServerError, w.Code)
		
		var resp ErrorResponse
		err := json.Unmarshal(w.Body.Bytes(), &resp)
		assert.NoError(t, err)
		assert.Equal(t, errors.CodeInternal, resp.Error.Code)
		assert.Equal(t, "An internal server error occurred", resp.Error.Message)
	})
}
