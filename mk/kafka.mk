
KAFKA_IMAGE := localhost/kafka:latest
ZOOKEEPER_OPTS ?= -Dzookeeper.4lw.commands.whitelist=*
KAFKA_OPTS ?= -Dzookeeper.4lw.commands.whitelist=*
ZOOKEEPER_CLIENT_PORT ?= 2181

# https://kafka.apache.org/quickstart

.PHONY: kafka-start
kafka-up: DOCKER_IMAGE=$(KAFKA_IMAGE)
kafka-up:  ## Start local kafka containers
	[ -e "$(KAFKA_DATA_DIR)" ] || mkdir -p "$(KAFKA_DATA_DIR)"
	$(DOCKER) container inspect kafka &> /dev/null || $(DOCKER) run \
	  -d \
	  --rm \
	  --name zookeeper \
	  -e ZOOKEEPER_CLIENT_PORT=$(ZOOKEEPER_CLIENT_PORT) \
	  -e ZOOKEEPER_OPTS="$(ZOOKEEPER_OPTS)" \
	  -v $(PWD)/kafka/data:/tmp/zookeeper:z \
	  -v $(PWD)/kafka/config:/tmp/config:z \
	  -p 8778:8778 \
	  -p 9092:9092 \
	--health-cmd /opt/kafka/scripts/zookeeper-healthcheck.sh \
	--health-interval 5s \
	--health-retries 10 \
	--health-timeout 3s \
	--health-start-period 3s \
	  "$(DOCKER_IMAGE)" \
	  /opt/kafka/scripts/zookeeper-entrypoint.sh

	$(DOCKER) container inspect kafka &> /dev/null || $(DOCKER) run \
	  -d \
	  --rm \
	  --name kafka \
	  --net container:zookeeper \
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
	  /opt/kafka/scripts/kafka-entrypoint.sh

.PHONY: kafka-stop  ## Stop local kafka containers
kafka-down: DOCKER_IMAGE=$(KAFKA_IMAGE)
kafka-down:  ## Stop local kafka infra
	! $(DOCKER) container inspect kafka &> /dev/null || $(DOCKER) container stop kafka
	! $(DOCKER) container inspect zookeeper &> /dev/null || $(DOCKER) container stop zookeeper

.PHONY: kafka-cli
kafka-cli:  ## Open an interactive shell in kafka container
	! $(DOCKER) container inspect kafka &> /dev/null || $(DOCKER) exec -it --workdir /opt/kafka/bin kafka /bin/bash

.PHONY: kafka-build
kafka-build: DOCKER_IMAGE=$(KAFKA_IMAGE)
kafka-build: DOCKER_DOCKERFILE=$(PROJECT_DIR)/kafka/Dockerfile
kafka-build:   ## Build local kafka container image
	$(DOCKER) build $(DOCKER_BUILD_OPTS) -t "$(DOCKER_IMAGE)" $(DOCKER_CONTEXT_DIR) -f "$(DOCKER_DOCKERFILE)"
