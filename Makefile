SHELL := /bin/bash
.DEFAULT_GOAL := help
COMPOSE := docker compose

.PHONY: help doctor up down logs migrate seed dev-api dev-web build test check fmt lint-go lint-web

help: ## Lista dostepnych komend
	@grep -hE '^[a-zA-Z_-]+:.*?## ' $(MAKEFILE_LIST) | awk 'BEGIN{FS=":.*?## "}{printf "  \033[36m%-12s\033[0m %s\n", $$1, $$2}'

doctor: ## Sprawdza wymagane narzedzia
	@bash scripts/doctor.sh

up: ## Postgres + Redis + Mailpit
	$(COMPOSE) up -d
	@echo "Mailpit UI: http://localhost:8025"

down: ## Zatrzymuje kontenery
	$(COMPOSE) down

logs: ## Logi kontenerow
	$(COMPOSE) logs -f

migrate: ## Stosuje migracje SQL z backend/migrations
	@bash scripts/migrate.sh

seed: ## Wgrywa fikcyjne dane demonstracyjne
	@bash scripts/seed.sh

dev-api: ## API na :8080
	cd backend && go run ./cmd/api

dev-web: ## UI na :5173
	cd frontend && npm run dev

build: ## Buduje backend i frontend
	cd backend && go build -o bin/api ./cmd/api
	cd frontend && npm run build

test: ## Testy backendu i frontendu
	cd backend && go test ./...
	cd frontend && npm run test

check: lint-go lint-web ## Wszystkie kontrole (to samo co CI)

lint-go:
	cd backend && gofmt -l . && go vet ./...
	cd backend && (command -v golangci-lint >/dev/null && golangci-lint run || echo "golangci-lint niezainstalowany - pomijam")

lint-web:
	cd frontend && npm run check && npm run lint

fmt: ## Formatowanie kodu
	cd backend && gofmt -w .
	cd frontend && npm run format
