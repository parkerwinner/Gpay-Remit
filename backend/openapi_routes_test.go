package main

import (
	"net/url"
	"regexp"
	"sort"
	"strings"
	"testing"

	"github.com/Gpay-Remit/gpay-remit/config"
	"github.com/Gpay-Remit/gpay-remit/handlers"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

// TestOpenAPISpecMatchesRoutes fails when the canonical OpenAPI spec
// (backend/handlers/openapi.yaml, the file served at /api/docs/openapi.yaml)
// and the Gin route table disagree: a route registered in code but not
// documented, or a documented operation with no route.
//
// Spec paths are relative to the document's `servers` (/api/v1 and /api/v2),
// unless a path declares its own `servers` (the root-mounted /health probes).
// Every documented operation must therefore exist under every server prefix.
func TestOpenAPISpecMatchesRoutes(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	registerRoutes(router, nil, nil, &config.Config{})

	actual := map[string]bool{}
	for _, r := range router.Routes() {
		actual[r.Method+" "+r.Path] = true
	}

	for route := range undocumentedRoutes {
		require.Truef(t, actual[route], "undocumentedRoutes lists %q, which is no longer registered; remove it", route)
		delete(actual, route)
	}

	expected := specRoutes(t, handlers.OpenAPISpec())

	var missingFromSpec, missingFromCode []string
	for r := range actual {
		if !expected[r] {
			missingFromSpec = append(missingFromSpec, r)
		}
	}
	for r := range expected {
		if !actual[r] {
			missingFromCode = append(missingFromCode, r)
		}
	}
	sort.Strings(missingFromSpec)
	sort.Strings(missingFromCode)

	if len(missingFromSpec) > 0 {
		t.Errorf("routes registered in main.go but missing from backend/handlers/openapi.yaml:\n  %s",
			strings.Join(missingFromSpec, "\n  "))
	}
	if len(missingFromCode) > 0 {
		t.Errorf("operations in backend/handlers/openapi.yaml with no registered route:\n  %s",
			strings.Join(missingFromCode, "\n  "))
	}
}

// undocumentedRoutes are infrastructure endpoints that are intentionally not
// part of the REST API contract.
var undocumentedRoutes = map[string]bool{
	"GET /metrics":               true, // Prometheus scrape endpoint
	"GET /api/docs":              true, // Swagger UI for this spec
	"GET /api/docs/openapi.yaml": true, // this spec itself
	"POST /graphql":              true, // GraphQL has its own schema
	"GET /graphql":               true, // GraphQL playground
	"GET /graphql/playground":    true,
	"GET /playground":            true,
}

var specParamRe = regexp.MustCompile(`\{([^}/]+)\}`)

type specServer struct {
	URL string `yaml:"url"`
}

type specDoc struct {
	Servers []specServer                    `yaml:"servers"`
	Paths   map[string]map[string]yaml.Node `yaml:"paths"`
}

var httpMethods = map[string]string{
	"get": "GET", "post": "POST", "put": "PUT", "patch": "PATCH",
	"delete": "DELETE", "head": "HEAD", "options": "OPTIONS",
}

// specRoutes expands every documented operation into "METHOD /full/gin/path".
func specRoutes(t *testing.T, data []byte) map[string]bool {
	t.Helper()
	var doc specDoc
	require.NoError(t, yaml.Unmarshal(data, &doc), "OpenAPI spec must be valid YAML")
	require.NotEmpty(t, doc.Servers, "OpenAPI spec must declare servers")
	require.NotEmpty(t, doc.Paths, "OpenAPI spec must declare paths")

	globalPrefixes := serverPrefixes(t, doc.Servers)
	out := map[string]bool{}
	for path, item := range doc.Paths {
		prefixes := globalPrefixes
		if node, ok := item["servers"]; ok {
			var servers []specServer
			require.NoErrorf(t, node.Decode(&servers), "servers of %s", path)
			prefixes = serverPrefixes(t, servers)
		}
		ginPath := specParamRe.ReplaceAllString(path, ":$1")
		for key := range item {
			method, ok := httpMethods[key]
			if !ok {
				continue // servers, parameters, summary, ...
			}
			for _, prefix := range prefixes {
				out[method+" "+prefix+ginPath] = true
			}
		}
	}
	return out
}

func serverPrefixes(t *testing.T, servers []specServer) []string {
	t.Helper()
	seen := map[string]bool{}
	var out []string
	for _, s := range servers {
		u, err := url.Parse(s.URL)
		require.NoErrorf(t, err, "server url %q", s.URL)
		p := strings.TrimRight(u.Path, "/")
		if !seen[p] {
			seen[p] = true
			out = append(out, p)
		}
	}
	return out
}

func TestSpecRoutesExpansion(t *testing.T) {
	spec := []byte(`
servers:
  - url: http://h/api/v1
  - url: http://h/api/v2/
paths:
  /things/{id}:
    parameters: []
    get: {}
    delete: {}
  /health:
    servers:
      - url: http://h
    get: {}
`)
	got := specRoutes(t, spec)
	require.Equal(t, map[string]bool{
		"GET /api/v1/things/:id":    true,
		"DELETE /api/v1/things/:id": true,
		"GET /api/v2/things/:id":    true,
		"DELETE /api/v2/things/:id": true,
		"GET /health":               true,
	}, got)
}
