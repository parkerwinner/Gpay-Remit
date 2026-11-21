package main

import (
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"

	"github.com/Gpay-Remit/gpay-remit/config"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// canonicalSpecPath is the single OpenAPI spec for the API (#379). It lives
// next to handlers/docs.go because it is embedded with go:embed and served at
// /api/docs/openapi.yaml, and go:embed cannot reach files outside the module.
var canonicalSpecPath = filepath.Join("handlers", "openapi.yaml")

// undocumentedRoutes are served by the router but deliberately left out of
// the REST spec. Each entry needs a reason; anything else the router serves
// must be documented.
var undocumentedRoutes = map[string]string{
	"GET /metrics":               "Prometheus scrape endpoint, not part of the public API",
	"GET /api/docs":              "Swagger UI page that renders this spec",
	"GET /api/docs/openapi.yaml": "serves this spec itself",
	"GET /graphql":               "GraphQL is described by its own schema, not OpenAPI",
	"POST /graphql":              "GraphQL is described by its own schema, not OpenAPI",
	"GET /playground":            "GraphQL playground UI",
	"GET /graphql/playground":    "GraphQL playground UI",
}

var httpMethods = map[string]bool{
	"get": true, "put": true, "post": true, "delete": true,
	"options": true, "head": true, "patch": true, "trace": true,
}

type routeSpecServer struct {
	URL string `yaml:"url"`
}

type routeSpec struct {
	Servers []routeSpecServer               `yaml:"servers"`
	Paths   map[string]map[string]yaml.Node `yaml:"paths"`
}

// serverPrefix is the path part of a server URL: "http://h/api/v1" -> "/api/v1".
func serverPrefix(t *testing.T, raw string) string {
	t.Helper()
	u, err := url.Parse(raw)
	require.NoError(t, err, "server url %q", raw)
	return strings.TrimRight(u.Path, "/")
}

// documentedRoutes expands the spec into "METHOD /full/path" keys. A path
// item's own `servers` overrides the top-level list, which is how routes
// mounted at the root (e.g. /health) are documented.
func documentedRoutes(t *testing.T, spec routeSpec) map[string]bool {
	t.Helper()
	out := map[string]bool{}
	for path, item := range spec.Paths {
		servers := spec.Servers
		if node, ok := item["servers"]; ok {
			servers = nil
			require.NoError(t, node.Decode(&servers), "servers of %s", path)
		}
		require.NotEmpty(t, servers, "%s has no server to resolve it against", path)
		for method := range item {
			if !httpMethods[method] {
				continue
			}
			for _, s := range servers {
				out[strings.ToUpper(method)+" "+serverPrefix(t, s.URL)+path] = true
			}
		}
	}
	return out
}

var ginParam = regexp.MustCompile(`[:*]([A-Za-z0-9_]+)`)

// registeredRoutes lists the router's routes with gin's :param and *param
// rewritten to OpenAPI's {param}.
func registeredRoutes(router *gin.Engine) map[string]bool {
	out := map[string]bool{}
	for _, r := range router.Routes() {
		out[r.Method+" "+ginParam.ReplaceAllString(r.Path, "{$1}")] = true
	}
	return out
}

func sortedMissing(have, want map[string]bool, skip map[string]string) []string {
	var missing []string
	for k := range want {
		if _, skipped := skip[k]; skipped {
			continue
		}
		if !have[k] {
			missing = append(missing, k)
		}
	}
	sort.Strings(missing)
	return missing
}

func routeTestConfig() *config.Config {
	return &config.Config{
		Environment:        "test",
		JWTSecret:          "route-test-secret-route-test-secret",
		HorizonURL:         "https://horizon-testnet.stellar.org",
		NetworkPassphrase:  "Test SDF Network ; September 2015",
		CORSAllowedOrigins: []string{"http://localhost:3000"},
		CORSAllowedMethods: []string{"GET", "POST", "PUT", "DELETE"},
	}
}

// TestOpenAPISpecMatchesRoutes fails when a route is served but not in the
// spec, or documented but not served (#379).
func TestOpenAPISpecMatchesRoutes(t *testing.T) {
	gin.SetMode(gin.TestMode)

	data, err := os.ReadFile(canonicalSpecPath)
	require.NoError(t, err)
	var spec routeSpec
	require.NoError(t, yaml.Unmarshal(data, &spec), "spec must be valid YAML")

	db, err := gorm.Open(sqlite.Open("file::memory:"), &gorm.Config{})
	require.NoError(t, err)
	router := setupRouter(routeTestConfig(), db)

	served := registeredRoutes(router)
	documented := documentedRoutes(t, spec)

	for route := range undocumentedRoutes {
		require.True(t, served[route], "undocumentedRoutes lists %q, which the router no longer serves; remove it", route)
		require.False(t, documented[route], "%q is documented, so it should not be in undocumentedRoutes", route)
	}

	notDocumented := sortedMissing(documented, served, undocumentedRoutes)
	notServed := sortedMissing(served, documented, nil)

	var msg strings.Builder
	if len(notDocumented) > 0 {
		fmt.Fprintf(&msg, "routes served but missing from %s:\n  %s\n", canonicalSpecPath, strings.Join(notDocumented, "\n  "))
	}
	if len(notServed) > 0 {
		fmt.Fprintf(&msg, "routes documented in %s but not served:\n  %s\n", canonicalSpecPath, strings.Join(notServed, "\n  "))
	}
	if msg.Len() > 0 {
		t.Fatal(msg.String())
	}
}

// TestOnlyOneOpenAPISpec guards against the duplicate spec coming back.
func TestOnlyOneOpenAPISpec(t *testing.T) {
	_, err := os.Stat(filepath.Join("..", "docs", "api", "openapi.yaml"))
	require.True(t, os.IsNotExist(err), "docs/api/openapi.yaml must not exist; %s is the canonical spec", canonicalSpecPath)
}
