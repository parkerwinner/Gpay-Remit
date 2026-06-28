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
