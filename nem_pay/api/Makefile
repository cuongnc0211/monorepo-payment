.PHONY: run migrate sqlc test test-db tidy

run:
	go run ./cmd/api

migrate:
	migrate -path db/migrations -database "$(DB_URL)" up

# sqlc runs via Docker so no host install is required (see repo toolchain decision).
sqlc:
	docker run --rm -v "$(CURDIR)":/src -w /src sqlc/sqlc:latest generate

test:
	go test ./...

# test-db spins up a throwaway Postgres, applies every up-migration, runs the full Go
# suite against it (tests that need a DB read TEST_DATABASE_URL; the rest run anyway), then
# tears the container down regardless of outcome.
#
# -p 1 runs packages one at a time: every DB-backed package TRUNCATEs shared tables for
# isolation, so running package test binaries in parallel would let them wipe each other's
# rows mid-test. Serialising packages keeps the single shared database consistent.
test-db:
	@CID=$$(docker run -d --rm -p 55432:5432 -e POSTGRES_PASSWORD=x -e POSTGRES_DB=nempay postgres:16-alpine); \
	trap "docker stop $$CID >/dev/null" EXIT; \
	for i in $$(seq 1 40); do docker exec $$CID psql -U postgres -d nempay -c 'SELECT 1' >/dev/null 2>&1 && break; sleep 1; done; \
	for f in db/migrations/*.up.sql; do docker exec -i $$CID psql -U postgres -d nempay -v ON_ERROR_STOP=1 -q < $$f; done; \
	TEST_DATABASE_URL="postgres://postgres:x@localhost:55432/nempay?sslmode=disable" go test -p 1 ./...

tidy:
	go mod tidy
