SHELL=bash
.PHONY: test
-include .env

clean:
	go clean
	rm dbmigrate

dbmigrate:
	go build -o dbmigrate cmd/dbmigrate/main.go

seed:
	go run cmd/dbmigrate/main.go seed


.env-export: .env
	sed -ne '/^export / {p;d}; /.*=/ s/^/export / p' .env > .env-export

test: .env-export
	. .env-export && go test ./...

test-ci:
	go test ./...

openapi:
	swag init --generalInfo api.go  --dir pkg/handler/
	#convert from swagger to openapi
	go run ./cmd/swagger2openapi/main.go docs/swagger.json docs/openapi.json
	rm docs/swagger.json docs/swagger.yaml
