.PHONY: test
-include .env

clean:
	go clean
	rm dbmigrate

dbmigrate:
	go build -o dbmigrate cmd/dbmigrate/main.go

seed:
	go run cmd/dbmigrate/main.go seed

test:	
	DATABASE_PASSWORD=$(DATABASE_PASSWORD) DATABASE_NAME=$(DATABASE_NAME) DATABASE_HOST=$(DATABASE_HOST) DATABASE_USER=$(DATABASE_USER) DATABASE_PORT=$(DATABASE_PORT) go test ./...

test-ci:
	go test ./...

openapi:
	swag init --generalInfo api.go  --dir pkg/handler/
	#convert from swagger to openapi
	go run ./cmd/swagger2openapi/main.go docs/swagger.json docs/openapi.json
	rm docs/swagger.json docs/swagger.yaml
