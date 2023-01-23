# zookeepr client port; it is not publised but used inter containers
ZOOKEEPER_CLIENT_PORT ?= 2181
export ZOOKEEPER_CLIENT_PORT
# The list of topics to be created; if more than one split them by a space
ifeq (,$(KAFKA_TOPICS))
	$(warning KAFKA_TOPICS is empty; probably missed definition at mk/variables.mk)
endif

KAFKA_COMPOSE_OPTIONS=KAFKA_CONFIG_DIR=$(KAFKA_CONFIG_DIR) \
						KAFKA_DATA_DIR=$(KAFKA_DATA_DIR) \
						ZOOKEEPER_CLIENT_PORT=$(ZOOKEEPER_CLIENT_PORT) \
						KAFKA_TOPICS=$(KAFKA_TOPICS) \

.PHONY: kafka-shell
kafka-shell:  ## Open an interactive shell in the kafka container
	$(COMPOSE_COMMAND) exec kafka /bin/bash

.PHONY: kafka-topics-list
kafka-topics-list:  ## List the kafka topics from the kafka container
	$(COMPOSE_COMMAND) exec kafka /opt/kafka/bin/kafka-topics.sh --list --bootstrap-server localhost:9092

.PHONY: kafka-topics-create
kafka-topics-create:  ## Create the kafka topics in KAFKA_TOPICS
	for topic in $(KAFKA_TOPICS); do \
	    $(COMPOSE_COMMAND) exec kafka /opt/kafka/bin/kafka-topics.sh --create --topic $$topic --bootstrap-server localhost:9092; \
	done

.PHONY: kafka-topics-describe
kafka-topics-describe:  ## Execute kafka-topics.sh for KAFKA_TOPICS
	for topic in $(KAFKA_TOPICS); do \
	    $(COMPOSE_COMMAND) exec kafka /opt/kafka/bin/kafka-topics.sh --describe --topic $$topic --bootstrap-server localhost:9092; \
	done

KAFKA_PROPERTIES ?= \
  --property print.key=true \
  --property print.partition=true \
  --property print.headers=true

.PHONY: kafka-topic-consume
kafka-topic-consume: KAFKA_TOPIC ?= $(firstword $(KAFKA_TOPICS))
kafka-topic-consume:  ## Execute kafka-console-consume.sh inside the kafka container for KAFKA_TOPIC (singular)
	@[ "$(KAFKA_TOPIC)" != "" ] || { echo "error:KAFKA_TOPIC cannot be empty"; exit 1; }
	$(COMPOSE_COMMAND) exec kafka \
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
	  -i --rm \
	  --net=host \
	  docker.io/edenhill/kcat:1.7.1 \
	  -k "$(KAFKA_MESSAGE_KEY)" \
	  -H X-Rh-Identity="$(KAFKA_IDENTITY)" \
	  -H X-Rh-Insight-Request-Id="$(KAFKA_REQUEST_ID)" \
	  -H Type="Introspect" \
	  -b localhost:9092 \
	  -t $(KAFKA_TOPIC) \
	  -P <<< "$$(cat "$(KAFKA_MESSAGE_FILE)" | jq -c -M )"
