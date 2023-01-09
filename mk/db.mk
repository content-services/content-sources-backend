##
# Set of rules to interact with a local database
# from a container and database initialization.
#
# Requires 'mk/docker.mk'
##
.PHONY: db-up
db-up: DOCKER_IMAGE=docker.io/postgres:14
db-up: RUN_MIGRATE ?= $(MAKE) db-migrate-up
db-up: $(GO_OUTPUT)/dbmigrate  ## Start postgres database (set empty RUN_MIGRATE to avoid db-migrate-up is launched)
	$(DOCKER) volume inspect $(DATABASE_CONTAINER_NAME) &> /dev/null || $(DOCKER) volume create $(DATABASE_CONTAINER_NAME)
	$(DOCKER) container inspect $(DATABASE_CONTAINER_NAME) &> /dev/null || $(DOCKER) run \
	  -d \
	  --rm \
	  --name $(DATABASE_CONTAINER_NAME) \
	  -p $(DATABASE_EXTERNAL_PORT):$(DATABASE_INTERNAL_PORT) \
	  -e POSTGRES_PASSWORD=$(DATABASE_PASSWORD) \
	  -e POSTGRES_USER=$(DATABASE_USER) \
	  -e POSTGRES_DB=$(DATABASE_NAME) \
	  -v $(DATABASE_CONTAINER_NAME):/var/lib/postgresql/data \
	  --health-cmd pg_isready \
	  --health-interval 5s \
	  --health-retries 10 \
	  --health-timeout 3s \
	  $(DOCKER_IMAGE)
	$(MAKE) .db-health-wait
	$(RUN_MIGRATE)
	@echo "Run 'make db-migrate-seed' to seed the database"

.PHONY: .db-health
.db-health:
	@echo -n "Checking database is ready: "
	@$(DOCKER) container inspect $(DATABASE_CONTAINER_NAME) &> /dev/null
	@$(DOCKER) exec $(DATABASE_CONTAINER_NAME) pg_isready

.PHONY: .db-health-wait
.db-health-wait:
	@$(DOCKER) container inspect $(DATABASE_CONTAINER_NAME) &> /dev/null
	@while [ "$$($(DOCKER) inspect -f '{{$(DOCKER_HEALTH_PATH)}}' $(DATABASE_CONTAINER_NAME) 2> /dev/null)" != "healthy" ]; do printf "."; sleep 1; done

.PHONY: db-migrate-up
db-migrate-up: $(GO_OUTPUT)/dbmigrate ## Run dbmigrate up
	$(GO_OUTPUT)/dbmigrate up

.PHONY: db-migrate-seed
db-migrate-seed: $(GO_OUTPUT)/dbmigrate ## Run dbmigrate seed
	$(GO_OUTPUT)/dbmigrate seed

.PHONY: db-down
db-down: ## Stop postgres database
	! $(DOCKER) container inspect $(DATABASE_CONTAINER_NAME) &> /dev/null || $(DOCKER) container stop $(DATABASE_CONTAINER_NAME)

.PHONY: db-clean
db-clean: db-down ## Clean database volume
	! $(DOCKER) volume inspect $(DATABASE_CONTAINER_NAME) &> /dev/null || $(DOCKER) volume rm $(DATABASE_CONTAINER_NAME)

.PHONY: db-cli-connect
db-cli-connect: ## Open a postgres cli in the container (it requires db-up)
	! $(DOCKER) container inspect $(DATABASE_CONTAINER_NAME) &> /dev/null || $(DOCKER) container exec -it $(DATABASE_CONTAINER_NAME) psql "sslmode=disable dbname=$(DATABASE_NAME) user=$(DATABASE_USER) host=$(DATABASE_HOST) port=$(DATABASE_INTERNAL_PORT) password=$(DATABASE_PASSWORD)"

.PHONY: db-dump-table
db-dump-table:
	! $(DOCKER) container inspect $(DATABASE_CONTAINER_NAME) &> /dev/null || $(DOCKER) container exec -it $(DATABASE_CONTAINER_NAME) pg_dump --table "$(DATABASE_TABLE)" --schema-only --dbname=$(DATABASE_NAME) --host=$(DATABASE_HOST) --port=$(DATABASE_INTERNAL_PORT) --username=$(DATABASE_USER)

.PHONY: db-shell
db-shell:
	! $(DOCKER) container inspect $(DATABASE_CONTAINER_NAME) &> /dev/null || $(DOCKER) container exec -it $(DATABASE_CONTAINER_NAME) bash
