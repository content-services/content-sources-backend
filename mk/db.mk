##
# Set of rules to interact with a local database
# from a container and database initialization.
#
# Requires 'mk/docker.mk'
##

.PHONY: db-up
db-up: DOCKER_IMAGE=docker.io/postgres:14
db-up: $(GO_OUTPUT)/dbmigrate  ## Start postgres database
	$(DOCKER) volume exists postgres || $(DOCKER) volume create postgres
	$(DOCKER) container exists postgres || $(DOCKER) run \
	  -d \
	  --rm \
	  --name postgres \
	  -p 5432:5432 \
	  -e POSTGRES_PASSWORD=$(DATABASE_PASSWORD) \
	  -e POSTGRES_USER=$(DATABASE_USER) \
	  -e POSTGRES_DB=$(DATABASE_NAME) \
	  -v postgres:/var/lib/postgresql/data \
	  --health-cmd pg_isready \
	  --health-interval 5s \
	  --health-retries 10 \
	  --health-timeout 3s \
	  $(DOCKER_IMAGE)
	$(MAKE) .db-health-wait
	$(MAKE) db-migrate-up
	@echo "Run 'make db-migrate-seed' to seed the database"

.PHONY: .db-health
.db-health:
	@echo -n "Checking database is ready: "
	@$(DOCKER) container exists postgres
	@$(DOCKER) exec postgres pg_isready

.PHONY: .db-health-wait
.db-health-wait:
	@$(DOCKER) container exists postgres
	@while [ "$$($(DOCKER) inspect -f '{{.State.Healthcheck.Status}}' postgres)" != "healthy" ]; do echo -n "."; sleep 1; done

.PHONY: db-migrate-up
db-migrate-up: $(GO_OUTPUT)/dbmigrate ## Run dbmigrate up
	$(GO_OUTPUT)/dbmigrate up

.PHONY: db-migrate-seed
db-migrate-seed: $(GO_OUTPUT)/dbmigrate ## Run dbmigrate seed
	$(GO_OUTPUT)/dbmigrate seed

.PHONY: db-down
db-down: ## Stop postgres database
	! $(DOCKER) container exists postgres || $(DOCKER) container stop postgres

.PHONY: db-clean
db-clean: db-down ## Clean database volume
	! $(DOCKER) volume exists postgres || $(DOCKER) volume rm postgres

.PHONY: db-cli-connect
db-cli-connect: ## Open a postgres cli in the container (it requires db-up)
	! $(DOCKER) container exists postgres || $(DOCKER) container exec -it postgres psql "sslmode=disable dbname=$(DATABASE_NAME) user=$(DATABASE_USER) host=$(DATABASE_HOST) port=$(DATABASE_PORT) password=$(DATABASE_PASSWORD)"
