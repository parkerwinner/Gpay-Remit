# Feature: Add database query logging in development

## Summary

Closes #250.

Slow queries were impossible to debug locally because GORM ran with its
default (silent) logger — there was no way to see which query ran, its
arguments, or how long it took. `backend/config/config.go` now builds an
explicit GORM logger via `InitDB`:

- **Development / any non-production environment** (`APP_ENV` unset or not
  `"production"`): `gormlogger.Info` level — every query is logged with its
  full SQL, bound parameters, row count, and execution time. Queries slower
  than `200ms` are flagged as slow.
- **Production** (`APP_ENV=production`): `gormlogger.Error` level — only
  query errors are logged, so production log volume and query-data exposure
  are unaffected.

Implementation notes:
- Uses GORM's own `gorm.io/gorm/logger` package (`logger.New(...)`) rather
  than a custom logger, per the ticket's request.
- Added a new `Config.Environment` field (sourced from `APP_ENV`, defaulting
  to `"development"`) so `InitDB` can make this decision without changing
  its signature.
- Log level selection is factored into a small `gormLogLevel(env string)`
  helper, unit tested directly.

## Changes

- `backend/config/config.go`
  - Added `Config.Environment`.
  - Added `gormLogLevel` / `newGormLogger` and wired the logger into
    `gorm.Open` inside `InitDB`.
- `backend/config/config_test.go`
  - Tests for `gormLogLevel` (dev/staging/empty → `Info`, production →
    `Error`) and that `newGormLogger` returns a usable logger for both.

## Test plan

- [x] `go build ./...`
- [x] `go vet ./...`
- [x] `go test ./...` (full suite, including `config` package)
- [x] Manual read-through confirming production behavior is unchanged
      (still error-only logging, same as GORM's prior default in practice)
