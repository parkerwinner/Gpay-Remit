# API specification

The OpenAPI spec lives in one place:
[`backend/handlers/openapi.yaml`](../../backend/handlers/openapi.yaml).

It is embedded in the backend binary and served at `GET /api/docs/openapi.yaml`.
Swagger UI is at `GET /api/docs`. `go:embed` cannot read files outside the Go
module, so the spec cannot live in this folder.

Do not add a copy here. `TestOnlyOneOpenAPISpec` fails if `docs/api/openapi.yaml`
comes back.

## Keeping the spec in sync with the routes

`backend/openapi_routes_test.go` builds the real Gin router (`setupRouter` in
`backend/router.go`). It then compares every route the router serves with
every operation in the spec, in both directions:

- **Served but not documented.** The test fails.
- **Documented but not served.** The test fails.
- **Health probes.** They live at the root, not under `/api/vN`, so they carry
  a path-level `servers` override. The test honours it.
- **Routes that should not be documented.** `/metrics`, the docs UI and
  GraphQL are listed in `undocumentedRoutes`, each with a reason.

The test runs in `ci.yml` as part of `go test ./...`, and in
`openapi-validation.yml` via `scripts/validate_openapi.sh`.

After adding or removing a route, update the spec and run:

```bash
cd backend && go test -run 'TestOpenAPI|TestOnlyOneOpenAPISpec' .
```
