package middleware

import (
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/Gpay-Remit/gpay-remit/errors"
	"github.com/gin-gonic/gin"
)

// API versioning policy (see docs/API_VERSIONING.md):
//
//   - SupportedAPIVersions is the closed set of versions the API serves.
//     Any other value — in the URL path, X-API-Version or Accept-Version —
//     is rejected with 400 UNSUPPORTED_API_VERSION. Unknown versions are
//     never silently mapped to the default.
//   - The version may be given in the path (/api/v2/...), X-API-Version or
//     Accept-Version. If more than one is given they must agree; a
//     mismatch is rejected with 400, because the request would otherwise be
//     routed to one version's handler while reporting another.
//   - With no version anywhere, DefaultAPIVersion is used.
//   - Deprecated versions keep working until their sunset date and carry
//     RFC 9745 `Deprecation`, RFC 8594 `Sunset` and a `Link` to the
//     migration guide on every response.
const (
	CurrentAPIVersion    = "v2"
	DeprecatedAPIVersion = "v1"
	DefaultAPIVersion    = CurrentAPIVersion

	// MigrationGuidePath is linked from deprecation headers.
	MigrationGuidePath = "/docs/migration"
)

// SupportedAPIVersions lists every version the API serves, oldest first.
var SupportedAPIVersions = []string{"v1", "v2"}

// deprecations holds the lifecycle dates of each deprecated version.
var deprecations = map[string]struct {
	deprecatedAt time.Time
	sunsetAt     time.Time
}{
	DeprecatedAPIVersion: {
		deprecatedAt: time.Date(2026, time.December, 31, 0, 0, 0, 0, time.UTC),
		sunsetAt:     time.Date(2027, time.June, 30, 0, 0, 0, 0, time.UTC),
	},
}

type VersionInfo struct {
	Version           string `json:"version"`
	IsDeprecated      bool   `json:"is_deprecated"`
	DeprecationDate   string `json:"deprecation_date,omitempty"`
	SunsetDate        string `json:"sunset_date,omitempty"`
	DeprecationNotice string `json:"deprecation_notice,omitempty"`
}

// pathVersionRe matches the version segment of /api/<version>/... paths:
// "v" followed by a digit (v1, v3, v2.1), so /api/vendors is not a version.
var pathVersionRe = regexp.MustCompile(`^/api/(v[0-9][^/]*)(?:/|$)`)

// versionSource records where a requested version came from.
type versionSource struct {
	name    string // "path", "X-API-Version" or "Accept-Version"
	raw     string // value as sent
	version string // normalized value
}

func VersionMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		sources := requestedVersions(c)

		requestedVersion := DefaultAPIVersion
		if len(sources) > 0 {
			requestedVersion = sources[0].version
		}

		for _, s := range sources {
			if !isValidVersion(s.version) {
				abortUnsupportedVersion(c, "Unsupported API version", gin.H{
					"requested_version":  s.raw,
					"source":             s.name,
					"supported_versions": SupportedAPIVersions,
				})
				return
			}
		}
		for _, s := range sources[min(1, len(sources)):] {
			if s.version != requestedVersion {
				abortUnsupportedVersion(c, "Conflicting API versions in request", gin.H{
					"versions":           versionsBySource(sources),
					"supported_versions": SupportedAPIVersions,
				})
				return
			}
		}

		c.Set("api_version", requestedVersion)

		if isDeprecatedVersion(requestedVersion) {
			setDeprecationHeaders(c, requestedVersion)
		}

		c.Header("X-API-Version", requestedVersion)
		c.Next()
	}
}

// requestedVersions returns every version the request names, path first.
// Blank headers are treated as absent.
func requestedVersions(c *gin.Context) []versionSource {
	var out []versionSource
	if m := pathVersionRe.FindStringSubmatch(c.Request.URL.Path); m != nil {
		out = append(out, versionSource{name: "path", raw: m[1], version: normalizeVersion(m[1])})
	}
	for _, h := range []string{"X-API-Version", "Accept-Version"} {
		if raw := c.GetHeader(h); strings.TrimSpace(raw) != "" {
			out = append(out, versionSource{name: h, raw: raw, version: normalizeVersion(raw)})
		}
	}
	return out
}

func versionsBySource(sources []versionSource) map[string]string {
	out := make(map[string]string, len(sources))
	for _, s := range sources {
		out[s.name] = s.raw
	}
	return out
}

func abortUnsupportedVersion(c *gin.Context, message string, details gin.H) {
	AbortWithError(c, http.StatusBadRequest, errors.CodeUnsupportedVersion, message, details)
}

func setDeprecationHeaders(c *gin.Context, version string) {
	d, ok := deprecations[version]
	if !ok {
		return
	}
	// Standard headers: RFC 9745 (structured-field date) and RFC 8594.
	c.Header("Deprecation", "@"+strconv.FormatInt(d.deprecatedAt.Unix(), 10))
	c.Header("Sunset", d.sunsetAt.Format(http.TimeFormat))
	c.Header("Link", "<"+MigrationGuidePath+`>; rel="deprecation"; type="text/html"`)

	// Legacy headers, kept for existing clients.
	c.Header("X-API-Deprecation-Warning", "This API version is deprecated")
	c.Header("X-API-Deprecation-Date", d.deprecatedAt.Format("2006-01-02"))
	c.Header("X-API-Sunset-Date", d.sunsetAt.Format("2006-01-02"))
	c.Header("X-API-Deprecation-Info", "Please migrate to "+CurrentAPIVersion+". See documentation at "+MigrationGuidePath)
}

func normalizeVersion(version string) string {
	version = strings.TrimSpace(strings.ToLower(version))
	if !strings.HasPrefix(version, "v") {
		version = "v" + version
	}
	return version
}

func isValidVersion(version string) bool {
	for _, v := range SupportedAPIVersions {
		if v == version {
			return true
		}
	}
	return false
}

func isDeprecatedVersion(version string) bool {
	_, ok := deprecations[version]
	return ok
}

// GetVersionInfo describes a supported version's lifecycle.
func GetVersionInfo(version string) VersionInfo {
	info := VersionInfo{Version: version}
	if d, ok := deprecations[version]; ok {
		info.IsDeprecated = true
		info.DeprecationDate = d.deprecatedAt.Format("2006-01-02")
		info.SunsetDate = d.sunsetAt.Format("2006-01-02")
		info.DeprecationNotice = "Please migrate to " + CurrentAPIVersion + ". See documentation at " + MigrationGuidePath
	}
	return info
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
			AbortWithError(c, http.StatusNotAcceptable, errors.CodeUnsupportedVersion,
				"This endpoint is not available in the requested API version", gin.H{
					"current_version":   currentVersion,
					"required_versions": allowedVersions,
				})
			return
		}

		c.Next()
	}
}
