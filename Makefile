.PHONY: run test race vet fmt migrate-up migrate-down

run:
	go run ./cmd/api

test:
	go test ./...

race:
	go test -race ./...

vet:
	go vet ./...

fmt:
	gofmt -w ./cmd

migrate-up:
	docker compose exec -T postgres psql -U postgres -d inventory < migrations/001_init.up.sql

migrate-down:
	docker compose exec -T postgres psql -U postgres -d inventory < migrations/001_init.down.sql
