.PHONY: test integration-test chaos-test coverage coverage-html check fmt-check

test:
	go test ./...

integration-test:
	go test -count=1 -tags=integration ./...

# Chaos scenarios run in-process by default and need no Docker.
# Pass TOXIPROXY=1 to additionally run the Toxiproxy-backed suite, which needs
# a proxy running (see backend/chaos/toxiproxy_test.go for the command).
chaos-test:
ifdef TOXIPROXY
	go test -race -count=1 -tags=toxiproxy ./chaos/...
else
	go test -race -count=1 ./chaos/...
endif

# Mirrors what CI runs, so a coverage regression is visible before pushing.
# -covermode=atomic is required with -race; count mode is not goroutine-safe.
coverage:
	go test -race -covermode=atomic -coverprofile=coverage.out ./...
	go tool cover -func=coverage.out | tail -n 1

coverage-html: coverage
	go tool cover -html=coverage.out -o coverage.html
	@echo "open backend/coverage.html"

# The gates CI applies, in the same order. Run before pushing.
#
# gofmt is deliberately not a gate here: a number of files predate any
# formatting check and reformatting them would bury a change in unrelated diff.
# `make fmt-check` reports them separately.
check:
	go vet ./...
	go build ./...
	go test ./...

fmt-check:
	@gofmt -l . || true
