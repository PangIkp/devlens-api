SHELL := /bin/bash

GO_IMAGE ?= golang:1.24
GO_RUN = docker run --rm -v "$(PWD):/workspace" -w /workspace/backend $(GO_IMAGE)
GO_TOOL_CACHE_DIR := $(PWD)/backend/.cache/go-build
GO_TOOL_MOD_CACHE_DIR := $(PWD)/backend/.cache/gomodcache
GO_TOOL_ENV = GOCACHE="$(GO_TOOL_CACHE_DIR)" GOMODCACHE="$(GO_TOOL_MOD_CACHE_DIR)"
POSTGRES_DSN ?= postgres://devlens:devlens@localhost:5432/devlens?sslmode=disable
ENV_FILE := $(PWD)/.env
LOAD_ENV = if [ -f "$(ENV_FILE)" ]; then set -a; source "$(ENV_FILE)"; set +a; fi;

.PHONY: fmt vet test tidy compose-config run migrate-up migrate-status sqlc-generate load-dashboard load-webhook

fmt:
	$(GO_RUN) gofmt -w .

vet:
	$(GO_RUN) go vet ./...

test:
	$(GO_RUN) go test ./...

tidy:
	$(GO_RUN) go mod tidy

compose-config:
	@$(LOAD_ENV) docker compose config

run:
	mkdir -p "$(GO_TOOL_CACHE_DIR)" "$(GO_TOOL_MOD_CACHE_DIR)"
	@$(LOAD_ENV) cd backend && $(GO_TOOL_ENV) go run ./cmd/api

migrate-up:
	mkdir -p "$(GO_TOOL_CACHE_DIR)" "$(GO_TOOL_MOD_CACHE_DIR)"
	@$(LOAD_ENV) cd backend && $(GO_TOOL_ENV) go run -tags "postgres" github.com/golang-migrate/migrate/v4/cmd/migrate@v4.19.0 -path db/migrations -database "$${POSTGRES_DSN:-$(POSTGRES_DSN)}" up

migrate-status:
	mkdir -p "$(GO_TOOL_CACHE_DIR)" "$(GO_TOOL_MOD_CACHE_DIR)"
	@$(LOAD_ENV) cd backend && POSTGRES_DSN="$${POSTGRES_DSN:-$(POSTGRES_DSN)}" $(GO_TOOL_ENV) go run ./cmd/migratestatus

sqlc-generate:
	mkdir -p "$(GO_TOOL_CACHE_DIR)" "$(GO_TOOL_MOD_CACHE_DIR)"
	cd backend && $(GO_TOOL_ENV) go run github.com/sqlc-dev/sqlc/cmd/sqlc@v1.30.0 generate

load-dashboard:
	mkdir -p "$(GO_TOOL_CACHE_DIR)" "$(GO_TOOL_MOD_CACHE_DIR)"
	@$(LOAD_ENV) cd backend && $(GO_TOOL_ENV) go run ./cmd/loadtest -scenario dashboard -repository-id "$${LOADTEST_REPOSITORY_ID}" -requests "$${LOADTEST_REQUESTS:-100}" -concurrency "$${LOADTEST_CONCURRENCY:-10}" -from "$${LOADTEST_FROM:-2026-08-01}" -to "$${LOADTEST_TO:-2026-08-13}" -page "$${LOADTEST_PAGE:-1}" -page-size "$${LOADTEST_PAGE_SIZE:-20}"

load-webhook:
	mkdir -p "$(GO_TOOL_CACHE_DIR)" "$(GO_TOOL_MOD_CACHE_DIR)"
	@$(LOAD_ENV) cd backend && $(GO_TOOL_ENV) go run ./cmd/loadtest -scenario webhook -requests "$${LOADTEST_REQUESTS:-100}" -concurrency "$${LOADTEST_CONCURRENCY:-10}" -webhook-event "$${LOADTEST_WEBHOOK_EVENT:-pull_request}" -webhook-body "$${LOADTEST_WEBHOOK_BODY:-{\"action\":\"opened\",\"repository\":{\"id\":42,\"full_name\":\"pangikp/devlens-api\"}}}"
