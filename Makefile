# ---- Settings ----
POSTGRES_DSN ?= postgres://casino:casino@localhost:5432/casino?sslmode=disable&x-migrations-table=auth_migrations

# Tell make to use bash, not sh (nicer error handling)
SHELL := /bin/bash

# ---- Phony targets (names that are NOT files) ----
.PHONY: help check start stop build test fmt run-auth \
        migrate-auth-up migrate-auth-down migrate-auth-version migrate-auth-create

# ---- Default target ----
help:
	@echo "Available targets:"
	@echo "  make check              - verify Postgres/Redis/RabbitMQ are up"
	@echo "  make start              - start local infra via systemd"
	@echo "  make stop               - stop local infra"
	@echo "  make build              - build all services"
	@echo "  make test               - run all tests"
	@echo "  make fmt                - format all Go code"
	@echo "  make run-api-gateway    - run api-gateway locally"
	@echo "  make run-auth           - run auth-service locally"
	@echo "  make migrate-auth-up    - apply auth migrations"
	@echo "  make migrate-auth-down  - roll back one auth migration"
	@echo "  make migrate-auth-version - print current auth schema version"
	@echo "  make migrate-auth-create NAME=add_foo - create a new migration pair"

# ---- Infra ----
check:
	./scripts/dev.sh

start:
	sudo systemctl start postgresql redis-server rabbitmq-server

stop:
	sudo systemctl stop postgresql redis-server rabbitmq-server

# ---- Go ----
build:
	@for mod in pkg services/*; do \
	  echo "==> $$mod"; \
	  (cd $$mod && go build ./...) || exit 1; \
	done

test:
	@set -e; for mod in pkg services/*; do \
	  echo "==> testing $$mod"; \
	  (cd $$mod && go test ./...); \
	done

fmt:
	@for mod in pkg services/*; do (cd $$mod && go fmt ./...); done

# ---- Run one service locally ----
# Usage: make run-api-gateway
run-api-gateway:
	cd services/api-gateway && \
	  go run .

# Usage: make run-auth
run-auth:
	cd services/auth-service && \
	  go run .

# ---- Migrations (auth) ----
migrate-auth-up:
	migrate -path migrations/auth -database "$(POSTGRES_DSN)" up 1

migrate-auth-down:
	migrate -path migrations/auth -database "$(POSTGRES_DSN)" down 1

migrate-auth-version:
	migrate -path migrations/auth -database "$(POSTGRES_DSN)" version

# Usage: make migrate-auth-create NAME=add_nickname
migrate-auth-create:
	@if [ -z "$(NAME)" ]; then echo "NAME is required, e.g. make migrate-auth-create NAME=add_nickname"; exit 1; fi
	migrate create -ext sql -dir migrations/auth -seq $(NAME)