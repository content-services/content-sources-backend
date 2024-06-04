##
# Rules to build openapi specification from the source code
# and to install swag locally.
##

SWAG=$(GO_OUTPUT)/swag
SWAG_VERSION := v1.8.4

.PHONY: install-swag
install-swag: $(SWAG) ## Install swag locally on your GO_OUTPUT (./release) directory

$(SWAG):
	@{\
		export GOPATH="$(shell mktemp -d "$(PROJECT_DIR)/tmp.XXXXXXXX" 2>/dev/null)" ; \
		echo "Using GOPATH='$${GOPATH}'" ; \
		[ "$${GOPATH}" != "" ] || { echo "error:GOPATH is empty"; exit 1; } ; \
		export GOBIN="$(dir $(SWAG))" ; \
		echo "Installing 'swag' at '$(SWAG)'" ; \
		go install github.com/swaggo/swag/cmd/swag@$(SWAG_VERSION) ; \
		find "$${GOPATH}" -type d -exec chmod u+w {} \; ; \
		rm -rf "$${GOPATH}" ; \
	}

.PHONY: openapi
openapi: install-swag ## Regenerate openapi json document and lint
	$(SWAG) init --generalInfo api.go --o ./api --dir pkg/handler/ --pd pkg/api
	# Convert from swagger to openapi
	go run ./cmd/swagger2openapi/main.go api/swagger.json api/openapi.json
	rm ./api/swagger.json ./api/swagger.yaml
	go run ./cmd/lint_openapi/main.go