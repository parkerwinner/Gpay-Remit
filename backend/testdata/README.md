# `backend/testdata/`

The Go toolchain **ignores any directory named `testdata`**. From `go help
packages`:

> Directory and file names that begin with "." or "_" are ignored by the go
> tool, as are directories named "testdata".

A Go *package* placed here has three consequences, two of them silent:

1. It is excluded from wildcard patterns, so `go test ./...` — which is what CI
   runs — never executes its tests.
2. `go mod tidy` does not scan it, so any dependency only imported from here is
   removed from `go.mod`, breaking the build for anything that imports it.
3. It is still importable by explicit path, which is why the breakage is not
   obvious until someone runs `tidy`.

The test data factories introduced for issue #255 therefore live in
[`backend/testfixtures/`](../testfixtures/) instead. The issue specified
`backend/testdata/factories.go`; that path would have produced a package CI
never runs and `go mod tidy` quietly breaks.

This directory remains available for its intended purpose: static fixture files
(golden JSON, sample payloads, certificates) loaded by tests at runtime.
