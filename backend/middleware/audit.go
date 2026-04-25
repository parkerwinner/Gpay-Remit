package middleware

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/yourusername/gpay-remit/models"
	"gorm.io/gorm"
)

const (
	auditOldKey = "audit_old"
	auditNewKey = "audit_new"
)

// SetAuditOld allows handlers to set a structured "before" snapshot.
func SetAuditOld(c *gin.Context, v interface{}) {
	if v == nil {
		return
	}
	if b, err := json.Marshal(v); err == nil {
		c.Set(auditOldKey, string(b))
	}
}

// SetAuditNew allows handlers to set a structured "after" snapshot.
func SetAuditNew(c *gin.Context, v interface{}) {
	if v == nil {
		return
	}
	if b, err := json.Marshal(v); err == nil {
		c.Set(auditNewKey, string(b))
	}
}

func normalizeJSONB(s string) string {
	if s == "" {
		return ""
	}
	b := []byte(s)
	if json.Valid(b) {
		return s
	}
	enc, err := json.Marshal(s)
	if err != nil {
		return ""
	}
	return string(enc)
}

// AuditTrail logs successful, state-changing requests (POST/PUT/PATCH/DELETE)
// into an append-only audit_logs table.
func AuditTrail(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		var requestBody []byte
		if c.Request != nil && c.Request.Body != nil && (c.Request.Method == http.MethodPost || c.Request.Method == http.MethodPut || c.Request.Method == http.MethodPatch) {
			requestBody, _ = io.ReadAll(c.Request.Body)
			c.Request.Body = io.NopCloser(bytes.NewBuffer(requestBody))
		}

		c.Next()

		if db == nil {
			return
		}
		if c.IsAborted() {
			return
		}
		if c.Writer == nil || c.Writer.Status() < 200 || c.Writer.Status() >= 400 {
			return
		}

		method := c.Request.Method
		if method != http.MethodPost && method != http.MethodPut && method != http.MethodPatch && method != http.MethodDelete {
			return
		}

		resource := c.FullPath()
		if resource == "" {
			resource = c.Request.URL.Path
		}

		var userID *uint
		if v, ok := c.Get("userID"); ok {
			if id, ok2 := v.(uint); ok2 {
				userID = &id
			}
		}

		oldValue, _ := c.Get(auditOldKey)
		newValue, _ := c.Get(auditNewKey)

		oldStr, _ := oldValue.(string)
		newStr, _ := newValue.(string)
		if newStr == "" && len(requestBody) > 0 {
			newStr = string(requestBody)
		}

		log := models.AuditLog{
			UserID:    userID,
			Action:    method,
			Resource:  resource,
			OldValue:  normalizeJSONB(oldStr),
			NewValue:  normalizeJSONB(newStr),
			IPAddress: c.ClientIP(),
		}

		_ = db.Create(&log).Error
	}
}
