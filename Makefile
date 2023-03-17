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

new-migration: # Create a new migration
	sql-migrate new -config ./db/dbconfig.yml $(shell bash -c 'read -p "Enter migration name: " name; echo $$name')

migrate: # Migrate the database to the latest version
	sql-migrate up -config ./db/dbconfig.yml
.PHONY: migrate

migrate-down: # Rollback the database one version down
	sql-migrate down -config ./db/dbconfig.yml
.PHONY: migrate-down

