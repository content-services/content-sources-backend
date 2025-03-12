
CS_COMPOSE_FILE ?= $(realpath deployments/docker-compose.yml)
PULP_COMPOSE_FILES ?= $(realpath compose_files/pulp/docker-compose.yml)

PULP_COMPOSE_OPTIONS=PULP_POSTGRES_PATH="pulp_db" PULP_STORAGE_PATH="pulp_storage"

PULP_COMPOSE_COMMAND=$(PULP_COMPOSE_OPTIONS) $(DOCKER)-compose --project-name=$(COMPOSE_PROJECT_NAME)  -f $(PULP_COMPOSE_FILES) up --detach
PULP_COMPOSE_DOWN_COMMAND=$(PULP_COMPOSE_OPTIONS) $(DOCKER)-compose --project-name=$(COMPOSE_PROJECT_NAME) -f $(PULP_COMPOSE_FILES) down

