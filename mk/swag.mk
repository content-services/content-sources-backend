##
# Rules to build openapi specification from the source code
# and to install swag locally.
##

SWAG=$(GO_OUTPUT)/swag
# version taken from go.mod, single source of truth, updated with dependabot
SWAG_VERSION := $(shell grep 'github.com/swaggo/swag ' go.mod | awk '{print $$2}')

# version taken from docker-compose.versions.yml
OPENAPI_GENERATOR_IMAGE := $(shell grep 'openapi-generator-cli' mk/docker-compose.versions.yml | awk -F': ' '{print $$2}')

.PHONY: install-swag
install-swag: $(SWAG) ## Install swag locally on your GO_OUTPUT (./release) directory

$(SWAG):
	@{\
		export GOPATH="$(shell mktemp -d "$(PROJECT_DIR)/tmp.XXXXXXXX" 2>/dev/null)" ; \
		echo "Using GOPATH='$${GOPATH}'" ; \
		[ "$${GOPATH}" != "" ] || { echo "error:GOPATH is empty"; exit 1; } ; \
		export GOBIN="$(dir $(SWAG))" ; \
		echo "Installing 'swag' at '$(SWAG)'" ; \
		go install github.com/swaggo/swag/cmd/swag@$(SWAG_VERSION) ; \
		find "$${GOPATH}" -type d -exec chmod u+w {} \; ; \
		rm -rf "$${GOPATH}" ; \
	}

.PHONY: openapi-doc
openapi-doc: install-swag ## Regenerate openapi json document and lint
	$(SWAG) init --generalInfo api.go --o ./api --dir pkg/handler/ --pd pkg/api
	# Convert from swagger to openapi
	go run ./cmd/swagger2openapi/main.go api/swagger.json api/openapi.json
	rm ./api/swagger.json ./api/swagger.yaml
	go run ./cmd/lint_openapi/main.go

.PHONY: openapi-js
openapi-js: 
	$(DOCKER) run -v .:/backend:z $(OPENAPI_GENERATOR_IMAGE) generate -i backend/api/openapi.json  -g typescript-fetch -o backend/_playwright-tests/test-utils/src/client

.PHONY: openapi
openapi: openapi-doc openapi-js
