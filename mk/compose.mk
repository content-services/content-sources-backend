##
# Set of rules to manage podman-compose
#
# Requires 'mk/db.mk'
# Requires 'mk/kafka.mk'
# Requires 'mk/pulp.mk'
##

# When 1 (default), pull images one service at a time before compose up to reduce Docker Hub burst rate limits (429).
COMPOSE_SERIAL_PULL ?= 1

# Service names from deployments/docker-compose.yml (pull order).
CONTENT_COMPOSE_PULL_SERVICES := postgres-content kafka redis-content candlepin minio

# Pulp stack: pull base images and one Pulp workload image before up (other Pulp services reuse the same image).
PULP_COMPOSE_PULL_SERVICES := postgres redis migration_service pulp_proxy

COMPOSE_COMMAND=$(DATABASE_COMPOSE_OPTIONS) \
                	$(KAFKA_COMPOSE_OPTIONS) \
                	$(DOCKER)-compose --project-name=$(COMPOSE_PROJECT_NAME) -f $(CS_COMPOSE_FILE)

.PHONY: .compose-pull-content
.compose-pull-content:
	@if [ "$(COMPOSE_SERIAL_PULL)" = 1 ]; then \
		for svc in $(CONTENT_COMPOSE_PULL_SERVICES); do \
			$(COMPOSE_COMMAND) pull $$svc || exit 1; \
		done; \
	fi

.PHONY: .compose-pull-pulp
.compose-pull-pulp:
	@if [ "$(COMPOSE_SERIAL_PULL)" = 1 ]; then \
		for svc in $(PULP_COMPOSE_PULL_SERVICES); do \
			$(PULP_COMPOSE_BASE) pull $$svc || exit 1; \
		done; \
	fi

.PHONY: .candlepin-health-wait
.candlepin-health-wait:
	@while [ $$($(DOCKER) ps | grep candlepin | grep healthy | wc -l) != 1 ]; do printf "."; sleep 1; done

.PHONY: compose-up
compose-up: $(GO_OUTPUT)/dbmigrate $(GO_OUTPUT)/candlepin ## Start up service dependencies using podman(docker)-compose
	./scripts/generate_pulp_certs.sh
	$(MAKE) .compose-pull-content
	$(COMPOSE_COMMAND) up --detach
	$(MAKE) .compose-pull-pulp
	$(PULP_COMPOSE_COMMAND)
	$(MAKE) .db-health-wait
	$(MAKE) db-migrate-up
	$(MAKE) .candlepin-health-wait
	@echo "Populating candlepin"
	$(GO_OUTPUT)/candlepin init
	@echo "Creating Topics"
	make kafka-topics-create

.PHONY: compose-run
compose-run: ## Start up service container dependencies, without initial data migrations.
	$(MAKE) .compose-pull-content
	$(COMPOSE_COMMAND) up --detach
	$(MAKE) .compose-pull-pulp
	$(PULP_COMPOSE_COMMAND)
	$(MAKE) .db-health-wait

.PHONY: compose-down
compose-down: ## Shut down service dependencies using podman(docker)-compose
	$(COMPOSE_COMMAND) down
	$(PULP_COMPOSE_DOWN_COMMAND)

.PHONY: compose-clean ## Clear out data (dbs, files) for service dependencies
compose-clean: compose-down
	if [ "$(DOCKER)" == "docker" ]; then \
		$(DOCKER) volume prune --force --all; \
	elif [ "$(DOCKER)" == "podman" ]; then \
		$(DOCKER) volume prune --force; \
	fi

.PHONY: compose-build
compose-build: ## Build service dependencies using podman(docker)-compose
	$(DOCKER)-compose --project-name=$(COMPOSE_PROJECT_NAME) -f $(CS_COMPOSE_FILE) build

