ADD_PYTHON_ENV := source .venv/bin/activate &&
GOLANGCI_LINT_VERSION := ""

.venv:
	python3 -m venv .venv && $(ADD_PYTHON_ENV) pip3 install -U pip

.PHONY: install-pre-commit
install-pre-commit: install-golangci-lint .venv ## Install pre-commit linter
	$(ADD_PYTHON_ENV) pip3 install pre-commit
	$(ADD_PYTHON_ENV) pre-commit install --install-hooks --allow-missing-config

.PHONY: install-golangci-lint
install-golangci-lint: ## Install golangci-lint Go linter
	curl -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/master/install.sh | sh -s -- -b $(GO_OUTPUT) $(GOLANGCI_LINT_VERSION)

.PHONY: lint
lint: ## Run Go linter
	$(GO_OUTPUT)/golangci-lint run --fix
