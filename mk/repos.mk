.PHONY: repos-download
repos-download: $(GO_OUTPUT)/external-repos  ## Download external repo urls from Image Builder
	{\
		export TMPDIR="$(shell mktemp -d)" ; \
		git clone https://github.com/osbuild/image-builder.git --sparse --depth=1 "$${TMPDIR}" \
		&& ( cd "$${TMPDIR}"; git sparse-checkout set distributions/ ) \
		&& $(GO_OUTPUT)/external-repos download --path "$${TMPDIR}/distributions/" \
	; }

.PHONY: repos-import
repos-import: ## Import External repo urls
	go run ./cmd/external-repos/main.go import

.PHONY: repos-import-rhel9
repos-import-rhel9: ## Import only rhel 9 repos
	OPTIONS_REPOSITORY_IMPORT_FILTER=rhel9 go run ./cmd/external-repos/main.go import

.PHONY: repos-import-rhel10
repos-import-rhel10: ## Import only rhel 10 repos
	OPTIONS_REPOSITORY_IMPORT_FILTER=rhel10 go run ./cmd/external-repos/main.go import

.PHONY: repos-minimal
repos-minimal: ## Import and snapshot or introspect repos needed before running Playwright tests, the repos used: HARDCODED (2) + SMALL + EPEL10
	OPTIONS_REPOSITORY_IMPORT_FILTER=hardcoded go run ./cmd/external-repos/main.go import
	go run cmd/external-repos/main.go snapshot --url https://cdn.redhat.com/content/dist/rhel10/10/aarch64/appstream/os/ --force
	OPTIONS_REPOSITORY_IMPORT_FILTER=hardcoded go run ./cmd/external-repos/main.go import
	go run cmd/external-repos/main.go snapshot --url https://cdn.redhat.com/content/dist/rhel10/10/aarch64/baseos/os/ --force
	OPTIONS_REPOSITORY_IMPORT_FILTER=small go run ./cmd/external-repos/main.go import
	go run cmd/external-repos/main.go snapshot --url https://cdn.redhat.com/content/dist/rhel9/9/aarch64/codeready-builder/os/ --force
	OPTIONS_REPOSITORY_IMPORT_FILTER=epel10 go run ./cmd/external-repos/main.go import
	go run cmd/external-repos/main.go introspect --url https://dl.fedoraproject.org/pub/epel/10/Everything/x86_64/ --force
