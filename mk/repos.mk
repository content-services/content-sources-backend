.PHONY: repos-download
repos-download: $(GO_OUTPUT)/external-repos  ## Download external repo urls from Image Builder
	{\
		export TMPDIR="$(shell mktemp -d)" ; \
		git clone https://github.com/osbuild/image-builder.git --sparse --depth=1 "$${TMPDIR}" \
		&& ( cd "$${TMPDIR}"; git sparse-checkout set distributions/ ) \
		&& $(GO_OUTPUT)/external-repos download "$${TMPDIR}/distributions/" \
	; }

.PHONY: repos-import
repos-import: ## Import External repo urls from Image Builders into the DB.  Generates pkg/external_repos/external_repos.json
	go run ./cmd/external-repos/main.go import
