.PHONY: conformance test

conformance:
	docker compose -f deploy/compose/docker-compose.yml up -d --build
	pytest -q tests/conformance
	docker compose -f deploy/compose/docker-compose.yml down -v

test:
	go vet ./...
	go test ./... -cover
	cd workers/python-adapter && poetry install && poetry run ruff check . && poetry run pytest -q
