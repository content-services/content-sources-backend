##
# General rules for interacting with container
# manager (podman or docker).
##

ifneq (,$(shell command podman -v 2>/dev/null))
DOCKER ?= podman
DOCKER_HEALTH_PATH ?= .State.Healthcheck.Status
DOCKER_DISABLE_SECURITY ?= --security-opt label=disable
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
DOCKER_LOGIN_USER ?= $(USER)
DOCKER_REGISTRY ?= quay.io
# DOCKER_BUILD_OPTS
# DOCKER_OPTS
# DOCKER_RUN_ARGS

.PHONY: docker-login
docker-login:
	$(DOCKER) login -u "$(DOCKER_LOGIN_USER)" -p "$(DOCKER_LOGIN_TOKEN)" $(DOCKER_REGISTRY)

.PHONY: docker-build
docker-build:  ## Build image DOCKER_IMAGE from DOCKER_DOCKERFILE using the DOCKER_CONTEXT_DIR
	# spooky action at a distance
	ln -f $(DOCKER_DOCKERFILE) Containerfile
	$(DOCKER) build $(DOCKER_BUILD_OPTS) -t "$(DOCKER_IMAGE)" $(DOCKER_CONTEXT_DIR)

.PHONY: docker-push
docker-push:  ## Push image to remote registry
	$(DOCKER) push "$(DOCKER_IMAGE)"

# TODO Indicate in the options the IP assigned to the postgres container
# .PHONY: docker-run
# docker-run: DOCKER_OPTS += --env-file .env
# docker-run:  ## Run with DOCKER_OPTS the DOCKER_IMAGE using DOCKER_RUN_ARGS as arguments (eg. make docker-run DOCKER_OPTS="-p 9000:9000")
# 	$(DOCKER) run $(DOCKER_OPTS) $(DOCKER_IMAGE) $(DOCKER_RUN_ARGS)
