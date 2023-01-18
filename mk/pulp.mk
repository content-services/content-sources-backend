CS_COMPOSE_FILE ?= "deployments/docker-compose.yml"
PULP_COMPOSE_FILES ?= "compose_files/pulp/pulp-oci-images/images/compose/docker-compose.yml"
PULP_COMPOSE_OPTIONS=PULP_POSTGRES_PATH="pulp_db" PULP_STORAGE_PATH="pulp_storage"

ifeq ($(DEPLOY_PULP),true)
PULP_COMPOSE_COMMAND=$(PULP_COMPOSE_OPTIONS) $(DOCKER)-compose --project-name=$(COMPOSE_PROJECT_NAME)  -f $(PULP_COMPOSE_FILES) up --detach
PULP_COMPOSE_DOWN_COMMAND=$(PULP_COMPOSE_OPTIONS) $(DOCKER)-compose --project-name=$(COMPOSE_PROJECT_NAME) -f $(PULP_COMPOSE_FILES) down
else
PULP_COMPOSE_COMMAND=echo "Skipping pulp deploy, set DEPLOY_PULP=true to deploy"
PULP_COMPOSE_DOWN_COMMAND=true
endif

compose_files/pulp/pulp-oci-images:
	git clone https://github.com/pulp/pulp-oci-images.git compose_files/pulp/pulp-oci-images


