help: # Show help for each of the Makefile recipes.
    @grep -E '^[a-zA-Z0-9 -]+:.*#'  Makefile | while read -r l; do printf "\033[1;32m$$(echo $$l | cut -f 1 -d':')\033[00m:$$(echo $$l | cut -f 2- -d'#')\n"; done
.PHONY: help

run: # Run the service (useful for local development)
	go run ./cmd/bot/main.go
.PHONY: run

build: # Build the service
	go build -o ./bin/bot ./cmd/bot/main.go

install-dev: # Install development dependencies
	go install github.com/rubenv/sql-migrate/...@latest

SQL_MIGRATE_CONFIG ?= ./db/dbconfig.yml
SQL_MIGRATE_ENV ?= development

new-migration: # Create a new migration
	sql-migrate new -config "${SQL_MIGRATE_CONFIG}" $(shell bash -c 'read -p "Enter migration name: " name; echo $$name')

migrate: # Migrate the database to the latest version
	sql-migrate up -config "${SQL_MIGRATE_CONFIG}" -env "${SQL_MIGRATE_ENV}"
.PHONY: migrate

migrate-down: # Rollback the database one version down
	sql-migrate down -config "${SQL_MIGRATE_CONFIG}" -env "${SQL_MIGRATE_ENV}"
.PHONY: migrate-down

