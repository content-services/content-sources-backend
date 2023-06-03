CS_COMPOSE_FILE ?= "deployments/docker-compose.yml"
PULP_COMPOSE_FILES ?= "compose_files/pulp/pulp-oci-images/images/compose/docker-compose.yml"
PULP_COMPOSE_OPTIONS=PULP_POSTGRES_PATH="pulp_db" PULP_STORAGE_PATH="pulp_storage"

PULP_COMPOSE_COMMAND=$(PULP_COMPOSE_OPTIONS) $(DOCKER)-compose --project-name=$(COMPOSE_PROJECT_NAME)  -f $(PULP_COMPOSE_FILES) up --detach
PULP_COMPOSE_DOWN_COMMAND=$(PULP_COMPOSE_OPTIONS) $(DOCKER)-compose --project-name=$(COMPOSE_PROJECT_NAME) -f $(PULP_COMPOSE_FILES) down

compose_files/pulp/pulp-oci-images:
	git clone https://github.com/pulp/pulp-oci-images.git compose_files/pulp/pulp-oci-images


