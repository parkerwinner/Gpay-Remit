package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
)

func TestVersionMiddleware(t *testing.T) {
	gin.SetMode(gin.TestMode)

	tests := []struct {
		name               string
		path               string
		headers            map[string]string
		expectedVersion    string
		expectedStatusCode int
		shouldHaveWarning  bool
	}{
		{
			name:               "Version from path v1",
			path:               "/api/v1/users",
			headers:            map[string]string{},
			expectedVersion:    "v1",
			expectedStatusCode: http.StatusOK,
			shouldHaveWarning:  true,
		},
		{
			name:               "Version from path v2",
			path:               "/api/v2/users",
			headers:            map[string]string{},
			expectedVersion:    "v2",
			expectedStatusCode: http.StatusOK,
			shouldHaveWarning:  false,
		},
		{
			name: "Version from header X-API-Version",
			path: "/api/users",
			headers: map[string]string{
				"X-API-Version": "v2",
			},
			expectedVersion:    "v2",
			expectedStatusCode: http.StatusOK,
			shouldHaveWarning:  false,
		},
		{
			name: "Version from header Accept-Version",
			path: "/api/users",
			headers: map[string]string{
				"Accept-Version": "v1",
			},
			expectedVersion:    "v1",
			expectedStatusCode: http.StatusOK,
			shouldHaveWarning:  true,
		},
		{
			name:               "Default version when not specified",
			path:               "/api/users",
			headers:            map[string]string{},
			expectedVersion:    "v2",
			expectedStatusCode: http.StatusOK,
			shouldHaveWarning:  false,
		},
		{
			name: "Invalid version",
			path: "/api/users",
			headers: map[string]string{
				"X-API-Version": "v99",
			},
			expectedVersion:    "",
			expectedStatusCode: http.StatusBadRequest,
			shouldHaveWarning:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			router := gin.New()
			router.Use(VersionMiddleware())
			router.GET("/*path", func(c *gin.Context) {
				c.JSON(http.StatusOK, gin.H{"status": "ok"})
			})

			req := httptest.NewRequest(http.MethodGet, tt.path, nil)
			for key, value := range tt.headers {
				req.Header.Set(key, value)
			}

			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			assert.Equal(t, tt.expectedStatusCode, w.Code)

			if tt.expectedStatusCode == http.StatusOK {
				assert.Equal(t, tt.expectedVersion, w.Header().Get("X-API-Version"))

				if tt.shouldHaveWarning {
					assert.NotEmpty(t, w.Header().Get("X-API-Deprecation-Warning"))
					assert.NotEmpty(t, w.Header().Get("X-API-Deprecation-Date"))
					assert.NotEmpty(t, w.Header().Get("X-API-Sunset-Date"))
				} else {
					assert.Empty(t, w.Header().Get("X-API-Deprecation-Warning"))
				}
			}
		})
	}
}

func TestGetAPIVersion(t *testing.T) {
	gin.SetMode(gin.TestMode)

	tests := []struct {
		name            string
		setVersion      bool
		versionValue    interface{}
		expectedVersion string
	}{
		{
			name:            "Version set in context",
			setVersion:      true,
			versionValue:    "v1",
			expectedVersion: "v1",
		},
		{
			name:            "Version not set in context",
			setVersion:      false,
			versionValue:    nil,
			expectedVersion: "v2",
		},
		{
			name:            "Invalid version type in context",
			setVersion:      true,
			versionValue:    123,
			expectedVersion: "v2",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			
			if tt.setVersion {
				c.Set("api_version", tt.versionValue)
			}

			version := GetAPIVersion(c)
			assert.Equal(t, tt.expectedVersion, version)
		})
	}
}

func TestRequireVersion(t *testing.T) {
	gin.SetMode(gin.TestMode)

	tests := []struct {
		name               string
		currentVersion     string
		requiredVersions   []string
		expectedStatusCode int
	}{
		{
			name:               "Version matches requirement",
			currentVersion:     "v2",
			requiredVersions:   []string{"v2"},
			expectedStatusCode: http.StatusOK,
		},
		{
			name:               "Version matches one of multiple requirements",
			currentVersion:     "v1",
			requiredVersions:   []string{"v1", "v2"},
			expectedStatusCode: http.StatusOK,
		},
		{
			name:               "Version does not match requirement",
			currentVersion:     "v1",
			requiredVersions:   []string{"v2"},
			expectedStatusCode: http.StatusNotAcceptable,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			router := gin.New()
			router.Use(func(c *gin.Context) {
				c.Set("api_version", tt.currentVersion)
				c.Next()
			})
			router.Use(RequireVersion(tt.requiredVersions...))
			router.GET("/test", func(c *gin.Context) {
				c.JSON(http.StatusOK, gin.H{"status": "ok"})
			})

			req := httptest.NewRequest(http.MethodGet, "/test", nil)
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			assert.Equal(t, tt.expectedStatusCode, w.Code)
		})
	}
}

func TestNormalizeVersion(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"v1", "v1"},
		{"V1", "v1"},
		{"1", "v1"},
		{" v2 ", "v2"},
		{"2", "v2"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := normalizeVersion(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestIsValidVersion(t *testing.T) {
	tests := []struct {
		version  string
		expected bool
	}{
		{"v1", true},
		{"v2", true},
		{"v3", false},
		{"v0", false},
		{"", false},
	}

	for _, tt := range tests {
		t.Run(tt.version, func(t *testing.T) {
			result := isValidVersion(tt.version)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestIsDeprecatedVersion(t *testing.T) {
	tests := []struct {
		version  string
		expected bool
	}{
		{"v1", true},
		{"v2", false},
	}

	for _, tt := range tests {
		t.Run(tt.version, func(t *testing.T) {
			result := isDeprecatedVersion(tt.version)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// #380: the version is validated against the supported set from every
// source, and deprecated versions carry standard deprecation headers.
func TestVersionMiddlewareValidation(t *testing.T) {
	gin.SetMode(gin.TestMode)

	serve := func(path string, headers map[string]string) *httptest.ResponseRecorder {
		router := gin.New()
		router.Use(VersionMiddleware())
		router.GET("/*path", func(c *gin.Context) {
			c.JSON(http.StatusOK, gin.H{"version": GetAPIVersion(c)})
		})
		req := httptest.NewRequest(http.MethodGet, path, nil)
		for k, v := range headers {
			req.Header.Set(k, v)
		}
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		return w
	}

	rejected := []struct {
		name    string
		path    string
		headers map[string]string
		message string
	}{
		{"unknown version in path", "/api/v3/remittances", nil, "Unsupported API version"},
		{"v0 in path", "/api/v0/remittances", nil, "Unsupported API version"},
		{"unknown version in Accept-Version", "/api/remittances", map[string]string{"Accept-Version": "v7"}, "Unsupported API version"},
		{"non-numeric version", "/api/remittances", map[string]string{"X-API-Version": "latest"}, "Invalid API version format"},
		{"minor version", "/api/remittances", map[string]string{"X-API-Version": "v2.0"}, "Invalid API version format"},
		{"injection attempt", "/api/remittances", map[string]string{"X-API-Version": "v2; DROP"}, "Invalid API version format"},
		{"header contradicts v1 path", "/api/v1/remittances", map[string]string{"X-API-Version": "v2"}, "does not match"},
		{"header contradicts v2 path", "/api/v2/remittances", map[string]string{"Accept-Version": "1"}, "does not match"},
	}
	for _, tt := range rejected {
		t.Run(tt.name, func(t *testing.T) {
			w := serve(tt.path, tt.headers)
			assert.Equal(t, http.StatusBadRequest, w.Code)
			assert.Contains(t, w.Body.String(), tt.message)
			assert.Contains(t, w.Body.String(), `"supported_versions":["v1","v2"]`)
			assert.NotContains(t, w.Body.String(), "DROP", "the raw header is never echoed")
		})
	}

	accepted := []struct {
		name    string
		path    string
		headers map[string]string
		version string
	}{
		{"matching header and path", "/api/v1/remittances", map[string]string{"X-API-Version": "V1"}, "v1"},
		{"bare number header", "/api/remittances", map[string]string{"Accept-Version": "2"}, "v2"},
		{"blank header is ignored", "/api/remittances", map[string]string{"X-API-Version": "   "}, "v2"},
		{"X-API-Version wins over Accept-Version", "/api/remittances", map[string]string{"X-API-Version": "v1", "Accept-Version": "v2"}, "v1"},
		{"root route keeps the default", "/health", nil, "v2"},
	}
	for _, tt := range accepted {
		t.Run(tt.name, func(t *testing.T) {
			w := serve(tt.path, tt.headers)
			assert.Equal(t, http.StatusOK, w.Code, w.Body.String())
			assert.Equal(t, tt.version, w.Header().Get("X-API-Version"))
			assert.Contains(t, w.Header().Get("Vary"), "X-API-Version")
		})
	}

	t.Run("deprecated v1 sends RFC 9745 Deprecation, RFC 8594 Sunset and a Link", func(t *testing.T) {
		w := serve("/api/v1/remittances", nil)
		assert.Equal(t, "@1798675200", w.Header().Get("Deprecation")) // 2026-12-31T00:00:00Z
		assert.Equal(t, "Wed, 30 Jun 2027 00:00:00 GMT", w.Header().Get("Sunset"))
		assert.Contains(t, w.Header().Get("Link"), `rel="deprecation"`)
		assert.Contains(t, w.Header().Get("Link"), "API_VERSIONING.md")
		assert.Equal(t, "2026-12-31", w.Header().Get("X-API-Deprecation-Date"))
	})

	t.Run("current v2 sends no deprecation headers", func(t *testing.T) {
		w := serve("/api/v2/remittances", nil)
		for _, h := range []string{"Deprecation", "Sunset", "Link", "X-API-Deprecation-Warning"} {
			assert.Empty(t, w.Header().Get(h), h)
		}
	})
}

func TestSupportedVersionsTable(t *testing.T) {
	assert.Equal(t, []string{"v1", "v2"}, SupportedVersions())
	info, ok := LookupVersion(CurrentAPIVersion)
	assert.True(t, ok)
	assert.False(t, info.IsDeprecated, "the current version cannot be deprecated")
	_, ok = LookupVersion(DefaultAPIVersion)
	assert.True(t, ok, "the default version must be supported")
	for _, v := range SupportedVersions() {
		info, _ := LookupVersion(v)
		if info.IsDeprecated {
			assert.NotEmpty(t, info.SunsetDate, "%s is deprecated, so it needs a sunset date", v)
		}
	}
}
