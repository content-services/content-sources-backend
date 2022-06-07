##
# Rules to build openapi specification from the source code
##

SWAG=$(GO_OUTPUT)/swag

$(SWAG):
	GOBIN=$(dir $(SWAG)) go install github.com/swaggo/swag/cmd/swag@latest

.PHONY: openapi
openapi: $(SWAG) ## Generate the openapi from the source code
	$(SWAG) init --generalInfo api.go --o ./api --dir pkg/handler/ --pd pkg/api
	# Convert from swagger to openapi
	go run ./cmd/swagger2openapi/main.go api/swagger.json api/openapi.json
	rm ./api/swagger.json ./api/swagger.yaml
