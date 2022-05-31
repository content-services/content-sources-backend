.PHONY: test
-include .env

clean:
	go clean
	rm release/*

content-sources:
	go build -o release/content-sources cmd/content-sources/main.go

dbmigrate:
	go build -o release/dbmigrate cmd/dbmigrate/main.go

seed:
	go run cmd/dbmigrate/main.go seed

test:
	CONFIG_PATH="$(shell pwd)/configs/" go test ./...

test-ci:
	go test ./...

openapi:
	swag init --generalInfo api.go  --dir pkg/handler/
	#convert from swagger to openapi
	go run ./cmd/swagger2openapi/main.go docs/swagger.json docs/openapi.json
	rm docs/swagger.json docs/swagger.yaml

arch:   #yum install plantuml if not installed
	java -jar /usr/share/java/plantuml.jar docs/architecture.puml

build: content-sources dbmigrate
	

image:
	podman build -f ./build/Dockerfile  ./
