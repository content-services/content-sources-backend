##
# sqlc code generation
##

SQLC ?= go run github.com/sqlc-dev/sqlc/cmd/sqlc@v1.29.0

.PHONY: sqlc-generate-lightwell
sqlc-generate-lightwell: ## Generate sqlc store for lightwell vulnerabilities
	cd "$(PROJECT_DIR)/pkg/lightwell/db" && $(SQLC) generate
