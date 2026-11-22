#!/usr/bin/env bash
set -euo pipefail

# Canonical spec: embedded into the backend and served at /api/docs/openapi.yaml.
SPEC_FILE="${1:-backend/handlers/openapi.yaml}"

echo "=========================================="
echo " Validating OpenAPI Specification: $SPEC_FILE"
echo "=========================================="

if [ ! -f "$SPEC_FILE" ]; then
    echo "❌ Error: OpenAPI specification file '$SPEC_FILE' not found!"
    exit 1
fi

# Run Go-based spec validation: structure, plus spec <-> Gin route parity.
echo "Running Go OpenAPI syntax, schema and route-parity verification..."
(cd backend && go mod download && go test -v -run 'TestOpenAPISpecValidation|TestOpenAPISpecMatchesRoutes|TestSpecRoutesExpansion' .)

# Verify frontend TypeScript types match OpenAPI spec
if command -v npx >/dev/null 2>&1; then
    echo "Verifying generated TypeScript types match OpenAPI spec..."
    npx --yes openapi-typescript "$SPEC_FILE" -o frontend/src/types/api.ts
    if ! git diff --exit-code frontend/src/types/api.ts >/dev/null 2>&1; then
        echo "❌ Error: frontend/src/types/api.ts is out of date. Run 'npm run generate:types' in frontend directory."
        exit 1
    fi
    echo "✅ TypeScript types are up to date."
fi

echo ""
echo "✅ OpenAPI Specification and types are valid and passed all validation checks!"
