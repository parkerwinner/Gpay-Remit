#!/usr/bin/env bash
set -euo pipefail

SPEC_FILE="${1:-docs/api/openapi.yaml}"

echo "=========================================="
echo " Validating OpenAPI Specification: $SPEC_FILE"
echo "=========================================="

if [ ! -f "$SPEC_FILE" ]; then
    echo "❌ Error: OpenAPI specification file '$SPEC_FILE' not found!"
    exit 1
fi

# Run Go-based spec validation test
echo "Running Go OpenAPI syntax and schema verification..."
cd backend && go test -v -run TestOpenAPISpecValidation .

echo ""
echo "✅ OpenAPI Specification is valid and passed all validation checks!"
