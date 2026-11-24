.PHONY: dev test

dev:
	docker compose -f docker-compose.yml -f docker-compose.dev.yml up --build

test:
	$(MAKE) -C backend test
