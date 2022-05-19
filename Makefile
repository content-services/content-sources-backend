TEST_DB_HOST:=
TEST_DB_PORT:=
TEST_DB_USER:=
TEST_DB_PASSWORD:=
TEST_DB_NAME:=


clean:
	go clean
	rm dbmigrate

dbmigrate:
	go build -o dbmigrate cmd/dbmigrate/main.go


test-all:
	export DB_HOST=$(TEST_DB_HOST) && \
	export DB_PORT=$(TEST_DB_PORT) && \
	export DB_USER=$(TEST_DB_USER) && \
	export DB_PASSWORD=$(TEST_DB_PASSWORD) && \
	export DB_NAME=$(TEST_DB_NAME) && \
	go test ./...
