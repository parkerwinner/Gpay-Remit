package middleware

import (
	"net/http"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

const (
	CurrentAPIVersion    = "v2"
	DeprecatedAPIVersion = "v1"
	DefaultAPIVersion    = CurrentAPIVersion

	// MigrationGuideURL is linked from the deprecation headers of every
	// deprecated version.
	MigrationGuideURL = "https://github.com/parkerwinner/Gpay-Remit/blob/main/docs/API_VERSIONING.md"
)

type VersionInfo struct {
	Version           string `json:"version"`
	IsDeprecated      bool   `json:"is_deprecated"`
	DeprecationDate   string `json:"deprecation_date,omitempty"`
	SunsetDate        string `json:"sunset_date,omitempty"`
	DeprecationNotice string `json:"deprecation_notice,omitempty"`
}

// supportedVersions is the single source of truth for which API versions the
// server accepts (#380). Adding a version, deprecating one or setting its
// sunset date happens here and nowhere else; validation, error messages and
// deprecation headers all read from this table. Dates are YYYY-MM-DD (UTC).
var supportedVersions = map[string]VersionInfo{
	"v1": {
		Version:           "v1",
		IsDeprecated:      true,
		DeprecationDate:   "2026-12-31",
		SunsetDate:        "2027-06-30",
		DeprecationNotice: "Please migrate to v2. See documentation at " + MigrationGuideURL,
	},
	"v2": {Version: "v2"},
}

// SupportedVersions lists the accepted versions in ascending order.
func SupportedVersions() []string {
	out := make([]string, 0, len(supportedVersions))
	for v := range supportedVersions {
		out = append(out, v)
	}
	sort.Strings(out)
	return out
}

// LookupVersion returns the metadata for a supported version.
func LookupVersion(version string) (VersionInfo, bool) {
	info, ok := supportedVersions[version]
	return info, ok
}

// pathVersion matches a version segment directly under /api/.
var pathVersion = regexp.MustCompile(`^/api/(v[0-9]+)(/|$)`)

// versionFormat is the shape every requested version must have after
// normalization: "v" followed by digits, nothing else.
var versionFormat = regexp.MustCompile(`^v[0-9]+$`)

func versionError(c *gin.Context, message string) {
	c.JSON(http.StatusBadRequest, gin.H{
		"error":              message,
		"supported_versions": SupportedVersions(),
		"default_version":    DefaultAPIVersion,
	})
	c.Abort()
}

// VersionMiddleware resolves the API version of every request and rejects
// anything it cannot serve with 400:
//
//   - a header (X-API-Version, then Accept-Version) or /api/vN/ path segment
//     naming a version that is not in supportedVersions
//   - a header that is not of the form vN / N (e.g. "v2.0", "latest")
//   - a header that contradicts the version in the path
//
// The resolved version is stored in the context ("api_version") and echoed
// in X-API-Version. Deprecated versions get RFC 9745 Deprecation, RFC 8594
// Sunset and a Link to the migration guide, plus the legacy X-API-* headers.
func VersionMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		// Responses differ by version, so caches must key on these headers.
		c.Writer.Header().Add("Vary", "X-API-Version, Accept-Version")

		fromPath, pathHasVersion := extractPathVersion(c.Request.URL.Path)
		fromHeader, headerHasVersion := extractHeaderVersion(c)

		if headerHasVersion && !versionFormat.MatchString(fromHeader) {
			versionError(c, "Invalid API version format; use vN, e.g. v2")
			return
		}

		requestedVersion := DefaultAPIVersion
		switch {
		case pathHasVersion && headerHasVersion && fromPath != fromHeader:
			versionError(c, "API version in header does not match the version in the URL path")
			return
		case pathHasVersion:
			requestedVersion = fromPath
		case headerHasVersion:
			requestedVersion = fromHeader
		}

		info, ok := LookupVersion(requestedVersion)
		if !ok {
			versionError(c, "Unsupported API version")
			return
		}

		c.Set("api_version", info.Version)

		if info.IsDeprecated {
			setDeprecationHeaders(c, info)
		}

		c.Header("X-API-Version", info.Version)
		c.Next()
	}
}

func setDeprecationHeaders(c *gin.Context, info VersionInfo) {
	if d, err := time.Parse("2006-01-02", info.DeprecationDate); err == nil {
		// RFC 9745: a Structured Field Date, "@" + Unix seconds.
		c.Header("Deprecation", "@"+strconv.FormatInt(d.UTC().Unix(), 10))
	} else {
		c.Header("Deprecation", "true")
	}
	if s, err := time.Parse("2006-01-02", info.SunsetDate); err == nil {
		// RFC 8594: an HTTP-date.
		c.Header("Sunset", s.UTC().Format(http.TimeFormat))
	}
	c.Header("Link", "<"+MigrationGuideURL+`>; rel="deprecation"; type="text/html"`)

	c.Header("X-API-Deprecation-Warning", "This API version is deprecated")
	if info.DeprecationDate != "" {
		c.Header("X-API-Deprecation-Date", info.DeprecationDate)
	}
	if info.SunsetDate != "" {
		c.Header("X-API-Sunset-Date", info.SunsetDate)
	}
	if info.DeprecationNotice != "" {
		c.Header("X-API-Deprecation-Info", info.DeprecationNotice)
	}
}

// extractPathVersion returns the vN segment of /api/vN/..., if present. It
// is returned even when unsupported, so an unknown /api/v9/ is rejected
// rather than silently served as the default version.
func extractPathVersion(path string) (string, bool) {
	m := pathVersion.FindStringSubmatch(path)
	if m == nil {
		return "", false
	}
	return m[1], true
}

// extractHeaderVersion reads X-API-Version, falling back to Accept-Version.
// A header that is blank after trimming counts as absent.
func extractHeaderVersion(c *gin.Context) (string, bool) {
	for _, name := range []string{"X-API-Version", "Accept-Version"} {
		if raw := strings.TrimSpace(c.GetHeader(name)); raw != "" {
			return normalizeVersion(raw), true
		}
	}
	return "", false
}

func normalizeVersion(version string) string {
	version = strings.TrimSpace(strings.ToLower(version))
	if !strings.HasPrefix(version, "v") {
		version = "v" + version
	}
	return version
}

func isValidVersion(version string) bool {
	_, ok := supportedVersions[version]
	return ok
}

func isDeprecatedVersion(version string) bool {
	info, ok := supportedVersions[version]
	return ok && info.IsDeprecated
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
				"error":             "This endpoint is not available in the requested API version",
				"current_version":   currentVersion,
				"required_versions": allowedVersions,
			})
			c.Abort()
			return
		}

		c.Next()
	}
}
