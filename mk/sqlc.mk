##
# sqlc code generation
##

# sqlc 1.31.1 needs Go 1.26; GOTOOLCHAIN=auto lets `go run` fetch it when setup-go pins local.
SQLC ?= go run github.com/sqlc-dev/sqlc/cmd/sqlc@v1.31.1

.PHONY: sqlc-generate-lightwell
sqlc-generate-lightwell: ## Generate sqlc store for lightwell vulnerabilities
	cd "$(PROJECT_DIR)/pkg/lightwell/db" && GOTOOLCHAIN=auto $(SQLC) generate
