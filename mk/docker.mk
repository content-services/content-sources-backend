##
# General rules for interacting with container
# manager (podman or docker).
##

ifneq (,$(shell command podman -v 2>/dev/null))
DOCKER ?= podman
DOCKER_HEALTH_PATH ?= .State.Healthcheck.Status
else
ifneq (,$(shell command docker -v 2>/dev/null))
DOCKER ?= docker
DOCKER_HEALTH_PATH ?= .State.Health.Status
else
DOCKER ?= false
endif
endif

DOCKER_CONTEXT_DIR ?= .
DOCKER_DOCKERFILE ?= Dockerfile
DOCKER_IMAGE_BASE ?= quay.io/$(USER)/myapp
DOCKER_IMAGE_TAG ?= $(shell git rev-parse --short HEAD)
DOCKER_IMAGE ?= $(DOCKER_IMAGE_BASE):$(DOCKER_IMAGE_TAG)
# DOCKER_OPTS
# DOCKER_RUN_ARGS

.PHONY: docker-build
docker-build:  ## Build image DOCKER_IMAGE from DOCKER_DOCKERFILE using the DOCKER_CONTEXT_DIR
	$(DOCKER) build -t "$(DOCKER_IMAGE)" -f $(DOCKER_DOCKERFILE) $(DOCKER_CONTEXT_DIR)

.PHONY: docker-push
docker-push:  ## Push image to remote registry
	$(DOCKER) push "$(DOCKER_IMAGE)"

# TODO Indicate in the options the IP assigned to the postgres container
# .PHONY: docker-run
# docker-run: DOCKER_OPTS += --env-file .env
# docker-run:  ## Run with DOCKER_OPTS the DOCKER_IMAGE using DOCKER_RUN_ARGS as arguments (eg. make docker-run DOCKER_OPTS="-p 9000:9000")
# 	$(DOCKER) run $(DOCKER_OPTS) $(DOCKER_IMAGE) $(DOCKER_RUN_ARGS)
