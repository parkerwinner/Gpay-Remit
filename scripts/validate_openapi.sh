#!/usr/bin/env bash
set -euo pipefail

# The canonical spec (#379). It is embedded into the binary and served at
# /api/docs/openapi.yaml, so it lives inside the Go module.
SPEC_FILE="${1:-backend/handlers/openapi.yaml}"

echo "=========================================="
echo " Validating OpenAPI Specification: $SPEC_FILE"
echo "=========================================="

if [ ! -f "$SPEC_FILE" ]; then
    echo "❌ Error: OpenAPI specification file '$SPEC_FILE' not found!"
    exit 1
fi

# Run Go-based spec validation test
echo "Running Go OpenAPI checks: syntax, required paths, and spec <-> Gin route parity..."
cd backend && go mod download && go test -v -run 'TestOpenAPISpecValidation|TestOpenAPISpecMatchesRoutes|TestOnlyOneOpenAPISpec' .

echo ""
echo "✅ OpenAPI Specification is valid and passed all validation checks!"
