##
# Set of rules to manage podman-compose
#
# Requires 'mk/db.mk'
# Requires 'mk/kafka.mk'
# Requires 'mk/pulp.mk'
##

COMPOSE_COMMAND=$(DATABASE_COMPOSE_OPTIONS) \
                	$(KAFKA_COMPOSE_OPTIONS) \
                	$(DOCKER)-compose --project-name=$(COMPOSE_PROJECT_NAME) -f $(CS_COMPOSE_FILE)

.PHONY: compose-up
compose-up: $(GO_OUTPUT)/dbmigrate compose_files/pulp/pulp-oci-images ## Start up service depdencies using podman(docker)-compose
	$(COMPOSE_COMMAND) up --detach
	$(PULP_COMPOSE_COMMAND)
	$(MAKE) .db-health-wait
	$(MAKE) db-migrate-up
	@echo "Run 'make db-migrate-seed' to seed the database"

.PHONY: compose-down
compose-down: ## Shut down service  depdencies using podman(docker)-compose
	$(COMPOSE_COMMAND) down
	$(PULP_COMPOSE_DOWN_COMMAND)

.PHONY: compose-clean ## Clear out data (dbs, files) for service dependencies
compose-clean: compose-down
	$(DOCKER) volume prune --force

.PHONY: compose-build
compose-build: ## Build service dependencies using podman(docker)-compose
	$(DOCKER)-compose --project-name=$(COMPOSE_PROJECT_NAME) -f $(CS_COMPOSE_FILE) build

