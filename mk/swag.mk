##
# Rules to build openapi specification from the source code
# and to install swag locally.
##

SWAG=$(GO_OUTPUT)/swag

.PHONY: install-swag
install-swag: $(SWAG) ## Install swag locally on your GO_OUTPUT (./release) directory

$(SWAG): GOPATH:=$(shell mktemp -d "$(PROJECT_DIR)/tmp.XXXXXXXX" 2>/dev/null)
$(SWAG):
	@{\
		echo "Using GOPATH='$(GOPATH)'" ; \
		export GOPATH="$(GOPATH)" ; \
		[ "$(GOPATH)" != "" ] || { echo "error:GOPATH is empty"; exit 1; } ; \
		export GOBIN="$(dir $(SWAG))" ; \
		echo "Installing 'swag' at '$(SWAG)'" ; \
		go install github.com/swaggo/swag/cmd/swag@latest ; \
		find "$(GOPATH)" -type d -exec chmod u+w {} \; ; \
		rm -rf "$(GOPATH)" ; \
	}

.PHONY: openapi
openapi: swag ## Generate the openapi from the source code
	$(SWAG) init --generalInfo api.go --o ./api --dir pkg/handler/ --pd pkg/api
	# Convert from swagger to openapi
	go run ./cmd/swagger2openapi/main.go api/swagger.json api/openapi.json
	rm ./api/swagger.json ./api/swagger.yaml
