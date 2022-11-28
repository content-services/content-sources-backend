##
# This file make easier to mock external clients by providing
# a static content for it.
##

MOCK_PID := $(PROJECT_DIR)/.mock
MOCK_PORT := 8800
MOCK_LOG := mock.log

CLIENTS_RBAC_BASE_URL ?= http://localhost:$(MOCK_PORT)

.PHONY: mock-up
mock-up: $(PROJECT_DIR)/release/mock ## Start mock service
	[ ! -e "$(MOCK_PID)" ] || ( echo "mock service is currently up"; exit 1 )
	"$(PROJECT_DIR)/release/mock" --directory "$(PROJECT_DIR)/test/data/mock" 8800 &> "$(MOCK_LOG)" & echo $$! > "$(MOCK_PID)"

.PHONY: mock-down
mock-down: ## Stop mock services
	[ -e "$(MOCK_PID)" ] || ( echo "mock service is not up currently"; exit 1 )
	/usr/bin/kill -SIGTERM "$$(<"$(MOCK_PID)")"
	[ ! -e "$(MOCK_PID)" ] || rm -f "$(MOCK_PID)"

.PHONY: mock-clean
mock-clean:  ## Clean the mock service
	[ ! -e "$(MOCK_PID)" ] || rm -f "$(MOCK_PID)"
	[ ! -e "$(MOCK_LOG)" ] || rm -f "$(MOCK_LOG)"
