##
# Set of rules to interact with a local database
# from a container and database initialization.
#
# Requires 'mk/docker.mk'
##

DATABASE_COMPOSE_OPTIONS=CONTENT_DATABASE_USER=$(DATABASE_USER) \
                         	CONTENT_DATABASE_PASSWORD=$(DATABASE_PASSWORD) \
                         	CONTENT_DATABASE_DATABASE_NAME=$(DATABASE_NAME) \
                         	CONTENT_DATABASE_PORT=$(DATABASE_EXTERNAL_PORT)

.PHONY: .db-health-wait
.db-health-wait:
	@while [ $($(COMPOSE_COMMAND) ps | grep postgres-content | grep healthy) 2> /dev/null ]; do printf "."; sleep 1; done

.PHONY: db-migrate-up
db-migrate-up: $(GO_OUTPUT)/dbmigrate ## Run dbmigrate up
	$(GO_OUTPUT)/dbmigrate up

.PHONY: db-migrate-seed
db-migrate-seed: $(GO_OUTPUT)/dbmigrate ## Run dbmigrate seed
	$(GO_OUTPUT)/dbmigrate seed

.PHONY: db-cli-connect
db-cli-connect: ## Open a postgres cli in the container (it requires db-up)
	$(COMPOSE_COMMAND) exec postgres-content psql "sslmode=disable dbname=$(DATABASE_NAME) user=$(DATABASE_USER) host=$(DATABASE_HOST) port=$(DATABASE_INTERNAL_PORT) password=$(DATABASE_PASSWORD)"

.PHONY: db-dump-table
db-dump-table:
	$(COMPOSE_COMMAND) exec postgres-content pg_dump --table "$(DATABASE_TABLE)" --schema-only --dbname=$(DATABASE_NAME) --host=$(DATABASE_HOST) --port=$(DATABASE_INTERNAL_PORT) --username=$(DATABASE_USER)

.PHONY: db-shell
db-shell:
	$(COMPOSE_COMMAND) exec postgres-content bash
