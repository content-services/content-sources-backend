##
# Golang rules to build the binaries, tidy dependencies,
# generate vendor directory, download dependencies and clean
# the generated binaries.
##

CONFIG_PATH ?= $(PROJECT_DIR)/configs/config.yaml
export CONFIG_PATH

# Directory where the built binaries will be generated
GO_OUTPUT ?= $(PROJECT_DIR)/bin
ifeq (,$(shell ls -1d vendor 2>/dev/null))
MOD_VENDOR :=
else
MOD_VENDOR ?= -mod vendor
endif

# Meta rule to add dependency on the binaries generated
.PHONY: build
build: $(patsubst cmd/%,$(GO_OUTPUT)/%,$(wildcard cmd/*)) ## Build binaries

# export CGO_ENABLED
# $(GO_OUTPUT)/%: CGO_ENABLED=0
$(GO_OUTPUT)/%: cmd/%/main.go
	@[ -e "$(GO_OUTPUT)" ] || mkdir -p "$(GO_OUTPUT)"
	go build $(MOD_VENDOR) -o "$@" "$<"

.PHONY: clean
clean: ## Clean binaries and testbin generated
	@[ ! -e "$(GO_OUTPUT)" ] || for item in cmd/*; do rm -vf "$(GO_OUTPUT)/$${item##cmd/}"; done

.PHONY: run
run: build ## Run the api & kafka consumer locally
	"$(GO_OUTPUT)/content-sources" api consumer instrumentation mock_rbac

.PHONY: tidy
tidy:
	go mod tidy

.PHONY: get-deps
get-deps: ## Download golang dependencies
	go get -d ./...

.PHONY: test
test: test-unit test-integration

.PHONY: test-unit
test-unit: ## Run tests for ci
	CONFIG_PATH="$(PROJECT_DIR)/configs/" go test $(MOD_VENDOR) ./pkg/...

.PHONY: test-integration
test-integration: ## Run tests for ci
	CONFIG_PATH="$(PROJECT_DIR)/configs/" go test $(MOD_VENDOR) ./test/integration/...

.PHONY: test-db-migrations
test-db-migrations: ## CI test for checking that db migrations work. Use carefully because it will drop the database.
	$(GO_OUTPUT)/dbmigrate up
	$(GO_OUTPUT)/dbmigrate down
	$(GO_OUTPUT)/dbmigrate up
	$(COMPOSE_COMMAND) exec postgres-content psql -c 'DROP TABLE schema_migrations;' "sslmode=disable dbname=$(DATABASE_NAME) user=$(DATABASE_USER) host=$(DATABASE_HOST) port=$(DATABASE_INTERNAL_PORT) password=$(DATABASE_PASSWORD)" &> /dev/null \
	|| psql -c 'DROP TABLE schema_migrations;' "sslmode=disable dbname=$(DATABASE_NAME) user=$(DATABASE_USER) host=$(DATABASE_HOST) port=$(DATABASE_INTERNAL_PORT) password=$(DATABASE_PASSWORD)"
	$(GO_OUTPUT)/dbmigrate up

# Add dependencies from binaries to all the the sources
# so any change is detected for the build rule
$(patsubst cmd/%,$(GO_OUTPUT)/%,$(wildcard cmd/*)): $(shell find $(PROJECT_DIR)/cmd -type f -name '*.go') $(shell find $(PROJECT_DIR)/pkg -type f -name '*.go')

