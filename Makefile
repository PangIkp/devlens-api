GO_IMAGE ?= golang:1.24
GO_RUN = docker run --rm -v "$(PWD):/workspace" -w /workspace/backend $(GO_IMAGE)
GO_TOOL_CACHE_DIR := $(PWD)/backend/.cache/go-build
GO_TOOL_MOD_CACHE_DIR := $(PWD)/backend/.cache/gomodcache
GO_TOOL_ENV = GOCACHE="$(GO_TOOL_CACHE_DIR)" GOMODCACHE="$(GO_TOOL_MOD_CACHE_DIR)"
POSTGRES_DSN ?= postgres://devlens:devlens@localhost:5432/devlens?sslmode=disable

.PHONY: fmt vet test tidy compose-config run migrate-up migrate-status sqlc-generate

fmt:
	$(GO_RUN) gofmt -w .

vet:
	$(GO_RUN) go vet ./...

test:
	$(GO_RUN) go test ./...

tidy:
	$(GO_RUN) go mod tidy

compose-config:
	docker compose config

run:
	cd backend && go run ./cmd/api

migrate-up:
	mkdir -p "$(GO_TOOL_CACHE_DIR)" "$(GO_TOOL_MOD_CACHE_DIR)"
	cd backend && $(GO_TOOL_ENV) go run -tags "postgres" github.com/golang-migrate/migrate/v4/cmd/migrate@v4.19.0 -path db/migrations -database "$(POSTGRES_DSN)" up

migrate-status:
	mkdir -p "$(GO_TOOL_CACHE_DIR)" "$(GO_TOOL_MOD_CACHE_DIR)"
	cd backend && POSTGRES_DSN="$(POSTGRES_DSN)" $(GO_TOOL_ENV) go run ./cmd/migratestatus

sqlc-generate:
	mkdir -p "$(GO_TOOL_CACHE_DIR)" "$(GO_TOOL_MOD_CACHE_DIR)"
	cd backend && $(GO_TOOL_ENV) go run github.com/sqlc-dev/sqlc/cmd/sqlc@v1.30.0 generate
