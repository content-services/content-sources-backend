##
# General rules for interacting with container
# manager (podman or docker).
##

ifneq (,$(shell command podman -v 2>/dev/null))
DOCKER ?= podman
else
ifneq (,$(shell command docker -v 2>/dev/null))
DOCKER ?= docker
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

.PHONY: docker-login
docker-login:
	$(DOCKER) login -u "$(DOCKER_LOGIN_USER)" -p "$(DOCKER_LOGIN_TOKEN)" $(DOCKER_REGISTRY)

.PHONY: docker-build
docker-build:  ## Build image DOCKER_IMAGE from DOCKER_DOCKERFILE using the DOCKER_CONTEXT_DIR
	$(DOCKER) build $(DOCKER_BUILD_OPTS) -t "$(DOCKER_IMAGE)" $(DOCKER_CONTEXT_DIR) -f "$(DOCKER_DOCKERFILE)"
.PHONY: docker-push
docker-push:  ## Push image to remote registry
	$(DOCKER) push "$(DOCKER_IMAGE)"