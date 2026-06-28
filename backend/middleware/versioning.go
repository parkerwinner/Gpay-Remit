package middleware

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

const (
	CurrentAPIVersion    = "v2"
	DeprecatedAPIVersion = "v1"
	DefaultAPIVersion    = CurrentAPIVersion
)

type VersionInfo struct {
	Version          string `json:"version"`
	IsDeprecated     bool   `json:"is_deprecated"`
	DeprecationDate  string `json:"deprecation_date,omitempty"`
	SunsetDate       string `json:"sunset_date,omitempty"`
	DeprecationNotice string `json:"deprecation_notice,omitempty"`
}

func VersionMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		requestedVersion := extractVersion(c)
		
		if requestedVersion == "" {
			requestedVersion = DefaultAPIVersion
		}

		if !isValidVersion(requestedVersion) {
			c.JSON(http.StatusBadRequest, gin.H{
				"error": "Invalid API version",
				"supported_versions": []string{"v1", "v2"},
			})
			c.Abort()
			return
		}

		c.Set("api_version", requestedVersion)

		if isDeprecatedVersion(requestedVersion) {
			c.Header("X-API-Deprecation-Warning", "This API version is deprecated")
			c.Header("X-API-Deprecation-Date", "2026-12-31")
			c.Header("X-API-Sunset-Date", "2027-06-30")
			c.Header("X-API-Deprecation-Info", "Please migrate to v2. See documentation at /docs/migration")
		}

		c.Header("X-API-Version", requestedVersion)
		c.Next()
	}
}

func extractVersion(c *gin.Context) string {
	version := c.GetHeader("X-API-Version")
	if version != "" {
		return normalizeVersion(version)
	}

	version = c.GetHeader("Accept-Version")
	if version != "" {
		return normalizeVersion(version)
	}

	path := c.Request.URL.Path
	if strings.HasPrefix(path, "/api/v1/") {
		return "v1"
	}
	if strings.HasPrefix(path, "/api/v2/") {
		return "v2"
	}

	return ""
}

func normalizeVersion(version string) string {
	version = strings.TrimSpace(strings.ToLower(version))
	if !strings.HasPrefix(version, "v") {
		version = "v" + version
	}
	return version
}

func isValidVersion(version string) bool {
	validVersions := map[string]bool{
		"v1": true,
		"v2": true,
	}
	return validVersions[version]
}

func isDeprecatedVersion(version string) bool {
	return version == DeprecatedAPIVersion
}

func GetAPIVersion(c *gin.Context) string {
	version, exists := c.Get("api_version")
	if !exists {
		return DefaultAPIVersion
	}
	if v, ok := version.(string); ok {
		return v
	}
	return DefaultAPIVersion
}

func RequireVersion(allowedVersions ...string) gin.HandlerFunc {
	return func(c *gin.Context) {
		currentVersion := GetAPIVersion(c)
		
		allowed := false
		for _, v := range allowedVersions {
			if currentVersion == v {
				allowed = true
				break
			}
		}

		if !allowed {
			c.JSON(http.StatusNotAcceptable, gin.H{
				"error": "This endpoint is not available in the requested API version",
				"current_version": currentVersion,
				"required_versions": allowedVersions,
			})
			c.Abort()
			return
		}

		c.Next()
	}
}
