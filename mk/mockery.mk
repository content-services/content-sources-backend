MOCKERY_VERSION := 2.36.1

export GO_OUTPUT

.PHONY: install-mockery
install-mockery: ## Install mockery locally on your GO_OUTPUT (./release) directory
	mkdir -p $(GO_OUTPUT) && \
	curl -sSfL https://github.com/vektra/mockery/releases/download/v$(MOCKERY_VERSION)/mockery_$(MOCKERY_VERSION)_$(shell uname -s)_$(shell uname -m).tar.gz | tar -xz -C $(GO_OUTPUT) mockery

.PHONY: mock
mock: ## Regenerate mocks
	go generate ./...
