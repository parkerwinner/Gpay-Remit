package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

type OpenAPISpec struct {
	OpenAPI    string                 `yaml:"openapi"`
	Info       map[string]interface{} `yaml:"info"`
	Paths      map[string]interface{} `yaml:"paths"`
	Components struct {
		Schemas         map[string]interface{} `yaml:"schemas"`
		SecuritySchemes map[string]interface{} `yaml:"securitySchemes"`
	} `yaml:"components"`
}

func TestOpenAPISpecValidation(t *testing.T) {
	specPath := filepath.Join("..", "docs", "api", "openapi.yaml")
	data, err := os.ReadFile(specPath)
	if err != nil {
		// try direct path
		specPath = filepath.Join("docs", "api", "openapi.yaml")
		data, err = os.ReadFile(specPath)
	}
	require.NoError(t, err, "Failed to read openapi.yaml")

	var spec OpenAPISpec
	err = yaml.Unmarshal(data, &spec)
	require.NoError(t, err, "OpenAPI spec should be valid YAML syntax")

	// 1. Verify OpenAPI version
	assert.True(t, strings.HasPrefix(spec.OpenAPI, "3."), "Spec version should be 3.x")

	// 2. Verify Info section
	assert.NotEmpty(t, spec.Info["title"], "info.title is required")
	assert.NotEmpty(t, spec.Info["version"], "info.version is required")

	// 3. Verify Required Paths exist
	requiredPaths := []string{
		"/auth/register",
		"/auth/login",
		"/auth/refresh",
		"/remittances",
		"/remittances/create",
		"/remittances/{id}",
		"/invoices",
		"/fees/calculate",
		"/contacts",
		"/contacts/{id}",
		"/contacts/{id}/verify",
		"/webhooks",
		"/health",
	}

	for _, path := range requiredPaths {
		assert.Contains(t, spec.Paths, path, "OpenAPI spec must contain path: %s", path)
	}

	// 4. Verify Schemas exist
	requiredSchemas := []string{
		"User",
		"Contact",
		"CreateContactRequest",
		"UpdateContactRequest",
		"CreateRemittanceRequest",
		"HealthResponse",
	}

	for _, schema := range requiredSchemas {
		assert.Contains(t, spec.Components.Schemas, schema, "Components.Schemas must contain schema: %s", schema)
	}
}
