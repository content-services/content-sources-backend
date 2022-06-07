##
# Golang rules to build the binaries, tidy dependencies,
# generate vendor directory and clean the generated binaries.
##

# Directory where the built binaries will be generated
GO_OUTPUT ?= $(PROJECT_DIR)/bin

.PHONY: build
build: vendor $(patsubst cmd/%,$(GO_OUTPUT)/%,$(wildcard cmd/*)) ## Build binaries

# export CGO_ENABLED
# $(GO_OUTPUT)/%: CGO_ENABLED=0
$(GO_OUTPUT)/%: cmd/%/main.go
	@[ -e "$(GO_OUTPUT)" ] || mkdir -p "$(GO_OUTPUT)"
	go build -mod vendor -o "$@" "$<"

.PHONY: clean
clean: ## Clean binaries and testbin generated
	@[ ! -e "$(GO_OUTPUT)" ] || for item in cmd/*; do rm -vf "$(GO_OUTPUT)/$${item##cmd/}"; done
#	@[ ! -e testbin ] || rm -rf testbin

.PHONY: run
run: build ## Run the service locally
	"$(GO_OUTPUT)/content-sources"

.PHONY: tidy
tidy:
	go mod tidy

.PHONY: vendor
vendor:
	go mod vendor

.PHONY: test
test: ## Run tests
	CONFIG_PATH="$(PROJECT_DIR)/configs/" go test -mod vendor ./...

.PHONY: test-ci
test-ci: ## Run tests for ci
	go test -mod vendor ./...
