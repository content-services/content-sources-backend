
# Kafka version to download when building the container
KAFKA_VERSION ?= 3.3.1
# The local image to use
KAFKA_IMAGE ?= localhost/kafka:latest
# Options passed to the jvm invokation for zookeeper container
ZOOKEEPER_OPTS ?= -Dzookeeper.4lw.commands.whitelist=*
# Options passed to the jvm invokation for kafka container
KAFKA_OPTS ?= -Dzookeeper.4lw.commands.whitelist=*
# zookeepr client port; it is not publised but used inter containers
ZOOKEEPER_CLIENT_PORT ?= 2181
# The list of topics to be created; if more than one split them by a space
ifeq (,$(KAFKA_TOPICS))
	$(warning KAFKA_TOPICS is empty; probably missed definition at mk/variables.mk)
endif
# KAFKA_TOPICS ?= platform.content-sources.introspect
KAFKA_GROUP_ID ?= content-sources

# The Kafka configuration directory that will be bound inside the containers
KAFKA_CONFIG_DIR ?= $(PROJECT_DIR)/kafka/config
# The Kafka data directory that will be bound inside the containers
# It must belong to the repository base directory
KAFKA_DATA_DIR ?= $(PROJECT_DIR)/kafka/data

# KAFKA_BOOTSTRAP_SERVERS ?= localhost:9092,localhost:9093
KAFKA_BOOTSTRAP_SERVERS ?= localhost:9092

# https://kafka.apache.org/quickstart
.PHONY: kafka-up
kafka-up: DOCKER_IMAGE=$(KAFKA_IMAGE)
kafka-up:  ## Start local kafka containers
	[ -e "$(KAFKA_DATA_DIR)" ] || mkdir -p "$(KAFKA_DATA_DIR)"
	$(DOCKER) container inspect kafka-zookeeper &> /dev/null || $(DOCKER) run \
	  -d \
	  --rm \
	  --name kafka-zookeeper \
	  -e ZOOKEEPER_CLIENT_PORT=$(ZOOKEEPER_CLIENT_PORT) \
	  -e ZOOKEEPER_OPTS="$(ZOOKEEPER_OPTS)" \
	  -v "$(KAFKA_DATA_DIR):/tmp/zookeeper:z" \
	  -v "$(KAFKA_CONFIG_DIR):/tmp/config:z" \
	  -p 8778:8778 \
	  -p 9092:9092 \
	  -p 2181:2181 \
	  --health-cmd /opt/kafka/scripts/zookeeper-healthcheck.sh \
	  --health-interval 5s \
	  --health-retries 10 \
	  --health-timeout 3s \
	  --health-start-period 3s \
	  "$(DOCKER_IMAGE)" \
	  /opt/kafka/scripts/zookeeper-entrypoint.sh
	$(DOCKER) container inspect kafka-broker &> /dev/null || $(DOCKER) run \
	  -d \
	  --rm \
	  --name kafka-broker \
	  --net container:kafka-zookeeper \
	  -e KAFKA_BROKER_ID=1 \
	  -e KAFKA_ZOOKEEPER_CONNECT="zookeeper:2181" \
	  -e KAFKA_ADVERTISED_LISTENERS="PLAINTEXT://kafka-broker:9092" \
	  -e ZOOKEEPER_CLIENT_PORT=$(ZOOKEEPER_CLIENT_PORT) \
	  -e KAFKA_LISTENER_SECURITY_PROTOCOL_MAP=PLAINTEXT:PLAINTEXT \
	  -e KAFKA_INTER_BROKER_LISTENER_NAME=PLAINTEXT \
	  -e KAFKA_OPTS='-javaagent:/usr/jolokia/agents/jolokia-jvm.jar=host=0.0.0.0' \
	  -e KAFKA_TOPICS="$(KAFKA_TOPICS)" \
	  -v "$(KAFKA_DATA_DIR):/tmp/zookeeper:z" \
	  -v "$(KAFKA_CONFIG_DIR):/tmp/config:z" \
	  "$(DOCKER_IMAGE)" \
	  /opt/kafka/scripts/kafka-entrypoint.sh

.PHONY: kafka-down
kafka-down: DOCKER_IMAGE=$(KAFKA_IMAGE)
kafka-down:  ## Stop local kafka infra
	! $(DOCKER) container inspect kafka-broker &> /dev/null || $(DOCKER) container stop kafka-broker
	! $(DOCKER) container inspect kafka-zookeeper &> /dev/null || $(DOCKER) container stop kafka-zookeeper
	$(DOCKER) container prune -f

.PHONY: kafka-clean
kafka-clean: kafka-down  ## Clean current local kafka infra
	export TMP="$(KAFKA_DATA_DIR)"; [ "$${TMP#$(PROJECT_DIR)/}" != "$${TMP}" ] \
	    || { echo "error:KAFKA_DATA_DIR should belong to $(PROJECT_DIR)"; exit 1; }
	rm -rf "$(KAFKA_DATA_DIR)"

.PHONY: kafka-shell
kafka-shell:  ## Open an interactive shell in the kafka container
	! $(DOCKER) container inspect kafka-broker &> /dev/null || $(DOCKER) exec -it --workdir /opt/kafka/bin kafka-broker /bin/bash

.PHONY: kafka-build
kafka-build: DOCKER_IMAGE=$(KAFKA_IMAGE)
kafka-build: DOCKER_DOCKERFILE=$(PROJECT_DIR)/kafka/Dockerfile
kafka-build: DOCKER_BUILD_OPTS+= --build-arg KAFKA_VERSION="$(KAFKA_VERSION)"
kafka-build:   ## Build local kafka container image
	@[ "$(KAFKA_VERSION)" != "" ] || { echo "error:KAFKA_VERSION is empty; check './scripts/kafka-print-last-version.py' output"; exit 1;}
	$(DOCKER) build $(DOCKER_BUILD_OPTS) -t "$(DOCKER_IMAGE)" $(DOCKER_CONTEXT_DIR) -f "$(DOCKER_DOCKERFILE)"

.PHONY: kafka-topics-list
kafka-topics-list:  ## List the kafka topics from the kafka container
	$(DOCKER) container inspect kafka-broker &> /dev/null || { echo "error:start kafka-broker container by 'make kafka-up'"; exit 1; }
	$(DOCKER) exec kafka-broker /opt/kafka/bin/kafka-topics.sh --list --bootstrap-server localhost:9092

.PHONY: kafka-topics-create
kafka-topics-create:  ## Create the kafka topics in KAFKA_TOPICS
	$(DOCKER) container inspect kafka-broker &> /dev/null || { echo "error:start kafka-broker container by 'make kafka-up'"; exit 1; }
	for topic in $(KAFKA_TOPICS); do \
	    $(DOCKER) exec kafka-broker /opt/kafka/bin/kafka-topics.sh --create --topic $$topic --bootstrap-server localhost:9092; \
	done

.PHONY: kafka-topics-describe
kafka-topics-describe:  ## Execute kafka-topics.sh for KAFKA_TOPICS
	$(DOCKER) container inspect kafka-broker &> /dev/null || { echo "error:start kafka-broker container by 'make kafka-up'"; exit 1; }
	for topic in $(KAFKA_TOPICS); do \
	    $(DOCKER) exec kafka-broker /opt/kafka/bin/kafka-topics.sh --describe --topic $$topic --bootstrap-server localhost:9092; \
	done

KAFKA_PROPERTIES ?= \
  --property print.key=true \
  --property print.partition=true \
  --property print.headers=true

.PHONY: kafka-topic-consume
kafka-topic-consume: KAFKA_TOPIC ?= $(firstword $(KAFKA_TOPICS))
kafka-topic-consume:  ## Execute kafka-console-consume.sh inside the kafka container for KAFKA_TOPIC (singular)
	@[ "$(KAFKA_TOPIC)" != "" ] || { echo "error:KAFKA_TOPIC cannot be empty"; exit 1; }
	$(DOCKER) exec kafka-broker \
	  /opt/kafka/bin/kafka-console-consumer.sh \
	  $(KAFKA_PROPERTIES) \
	  --topic $(KAFKA_TOPIC) \
	  --group $(KAFKA_GROUP_ID) \
	  --bootstrap-server localhost:9092

# https://stackoverflow.com/questions/58716683/is-there-a-way-to-add-headers-in-kafka-console-producer-sh
# https://github.com/edenhill/kcat
# https://dev.to/de_maric/learn-how-to-use-kafkacat-the-most-versatile-kafka-cli-client-1kb4
.PHONY: kafka-produce-msg
kafka-produce-msg: KAFKA_TOPIC ?= $(firstword $(KAFKA_TOPICS))
kafka-produce-msg: KAFKA_IDENTITY ?= eyJpZGVudGl0eSI6eyJ0eXBlIjoiQXNzb2NpYXRlIiwiYWNjb3VudF9udW1iZXIiOiIxMTExMTEiLCJpbnRlcm5hbCI6eyJvcmdfaWQiOiIyMjIyMjIifX19Cg==
kafka-produce-msg: KAFKA_REQUEST_ID ?= demo
kafka-produce-msg: KAFKA_MESSAGE_KEY ?= c67cd587-3741-493d-9302-f655fcd3bd68
kafka-produce-msg: KAFKA_MESSAGE_FILE ?= test/kafka/demo-introspect-request-1.json
kafka-produce-msg: ## Produce a demo kafka message to introspect
	$(DOCKER) run \
	  --net container:kafka-zookeeper \
	  -i --rm \
	  docker.io/edenhill/kcat:1.7.1 \
	  -k "$(KAFKA_MESSAGE_KEY)" \
	  -H X-Rh-Identity="$(KAFKA_IDENTITY)" \
	  -H X-Rh-Insight-Request-Id="$(KAFKA_REQUEST_ID)" \
	  -H Type="Introspect" \
	  -b localhost:9092 \
	  -t $(KAFKA_TOPIC) \
	  -P <<< "$$(cat "$(KAFKA_MESSAGE_FILE)" | jq -c -M )"
