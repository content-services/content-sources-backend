
KAFKA_IMAGE := localhost/kafka:latest
KAFKA_OPTS ?= -Dzookeeper.4lw.commands.whitelist=*
ZOOKEEPER_CLIENT_PORT ?= 2181

# https://kafka.apache.org/quickstart

.PHONY: kafka-start
kafka-up: DOCKER_IMAGE=$(KAFKA_IMAGE)
kafka-up:  ## Start local kafka infra
	# $(DOCKER) volume inspect kafka &> /dev/null || $(DOCKER) volume create kafka
	$(DOCKER) pod inspect kafka &> /dev/null \
	|| $(DOCKER) pod create \
	  --name kafka \
	  --hostname kafka.test \
	  -p "8778:8778" \
	  -p "9092:9092" \
	  -p $(ZOOKEEPER_CLIENT_PORT):$(ZOOKEEPER_CLIENT_PORT)
	$(DOCKER) container inspect zookeeper &> /dev/null || $(DOCKER) run \
	  -d \
	  --rm \
	  --pod kafka \
	  --name zookeeper \
	  -e ZOOKEEPER_CLIENT_PORT=$(ZOOKEEPER_CLIENT_PORT) \
	  -e KAFKA_OPTS="$(KAFKA_OPTS)" \
	  -v $(PWD)/kafka/data:/tmp/zookeeper:z \
	  -v $(PWD)/kafka/config:/tmp/config:z \
	  "$(DOCKER_IMAGE)" \
	  /opt/kafka/bin/zookeeper-server-start.sh /tmp/config/zookeeper.properties

	$(DOCKER) container inspect kafka &> /dev/null || $(DOCKER) run \
	  --rm \
	  --pod kafka \
	  --name kafka \
	  -e KAFKA_BROKER_ID=1 \
	  -e KAFKA_ZOOKEEPER_CONNECT="zookeeper:2181" \
	  -e KAFKA_ADVERTISED_LISTENERS="PLAINTEXT://kafka:9092" \
	  -e ZOOKEEPER_CLIENT_PORT=$(ZOOKEEPER_CLIENT_PORT) \
	  -e KAFKA_LISTENER_SECURITY_PROTOCOL_MAP=PLAINTEXT:PLAINTEXT \
	  -e KAFKA_INTER_BROKER_LISTENER_NAME=PLAINTEXT \
	  -e KAFKA_OPTS='-javaagent:/usr/jolokia/agents/jolokia-jvm.jar=host=0.0.0.0' \
	  -v $(PWD)/kafka/data:/tmp/zookeeper:z \
	  -v $(PWD)/kafka/config:/tmp/config:z \
	  "$(DOCKER_IMAGE)" \
	  /opt/kafka/bin/kafka-server-start.sh /tmp/config/server.properties

#   --health-cmd pg_isready \
#   --health-interval 5s \
#   --health-retries 10 \
#   --health-timeout 3s \

.PHONY: kafka-stop
kafka-down: DOCKER_IMAGE=$(KAFKA_IMAGE)
kafka-down:  ## Stop local kafka infra
	! $(DOCKER) container inspect zookeeper &> /dev/null || $(DOCKER) container stop zookeeper
	! $(DOCKER) container inspect kafka &> /dev/null || $(DOCKER) container stop kafka
	! $(DOCKER) pod inspect kafka &> /dev/null || $(DOCKER) pod rm kafka


.PHONY: kafka-build
kafka-build: DOCKER_IMAGE=$(KAFKA_IMAGE)
kafka-build: DOCKER_DOCKERFILE=$(PROJECTDIR)/kafka/Dockerfile
kafka-build:   ## Build local kafka container image
	$(DOCKER) build $(DOCKER_BUILD_OPTS) -t "$(DOCKER_IMAGE)" $(DOCKER_CONTEXT_DIR) -f "$(DOCKER_DOCKERFILE)"
