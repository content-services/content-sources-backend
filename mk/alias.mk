##
# This file contains different alias rules to keep
# compatibility with the previous rules.
##

.PHONY: arch
arch: docs/architecture.svg  ## Alias for 'make plantuml-generate'

.PHONY: image
image: DOCKER_DOCKERFILE := ./build/Dockerfile
image: DOCKER_IMAGE := content-sources:$(DOCKER_IMAGE_TAG)
image: ## Alias for 'make docker-build DOCKER_DOCKERFILE=build/Dockerfile DOCKER_IMAGE=content-sources:$(git rev-parse --short HEAD)
	$(MAKE) docker-build \
	  DOCKER_DOCKERFILE=$(DOCKER_DOCKERFILE) \
	  DOCKER_IMAGE=$(DOCKER_IMAGE)

.PHONY: seed
seed: db-migrate-seed ## Alias for 'make db-migrate-seed'

.PHONY: dbmigrate
dbmigrate: $(GO_OUTPUT)/dbmigrate  ## Alias for 'make build' for dbmigrate

.PHONY: content-sources
content-sources: $(GO_OUTPUT)/content-sources ## Alias for 'make build' for content-sources

.PHONY: swagger2openapi
swagger2openapi: $(GO_OUTPUT)/swagger2openapi ## Alias for 'make build' for swagger2openapi

