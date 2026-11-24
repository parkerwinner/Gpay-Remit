.PHONY: fmt fmt-check

# Format the Soroban contracts with rustfmt. The backend has its own Makefile
# (backend/Makefile); gofmt is intentionally not run over the whole tree here.
fmt:
	cd contracts && cargo fmt --all

# Same check the "Test Smart Contracts" CI job runs (`cargo fmt --check`).
fmt-check:
	cd contracts && cargo fmt --all -- --check
.PHONY: dev test

dev:
	docker compose -f docker-compose.yml -f docker-compose.dev.yml up --build

test:
	$(MAKE) -C backend test
