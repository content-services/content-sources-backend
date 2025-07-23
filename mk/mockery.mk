# MOCKERY_VERSION := $(shell curl -L https://api.github.com/repos/vektra/mockery/releases/latest | jq --raw-output .tag_name | sed 's/^v//')
# TODO: When the mockery config is updated to be v3 compatible. Remove the line bellow which pins the version to "v2.*" and uncomment the one above.
MOCKERY_VERSION := $(shell curl -L https://api.github.com/repos/vektra/mockery/tags\?per_page=1000 | grep "\"name\": \"v2." | head -n 1 | sed 's/.*"v\([^"]*\)".*/\1/')

export GO_OUTPUT

$(GO_OUTPUT)/mockery: ## Install mockery locally on your GO_OUTPUT (./release) directory
	mkdir -p $(GO_OUTPUT) && \
	curl -sSfL https://github.com/vektra/mockery/releases/download/v$(MOCKERY_VERSION)/mockery_$(MOCKERY_VERSION)_$(shell uname -s)_$(shell uname -m).tar.gz | tar -xz -C $(GO_OUTPUT) mockery

.PHONY: mock
mock: $(GO_OUTPUT)/mockery ## Install mockery if it isn't already in ./release directory and regenerate mocks
	./scripts/mockery_comment_fix.sh comment
	$(GO_OUTPUT)/mockery
	./scripts/mockery_comment_fix.sh uncomment
