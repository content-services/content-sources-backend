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
						KAFKA_TOPICS="$(KAFKA_TOPICS)" \

.PHONY: kafka-shell
kafka-shell:  ## Open an interactive shell in the kafka container
	$(COMPOSE_COMMAND) exec kafka /bin/bash

.PHONY: kafka-topics-list
kafka-topics-list:  ## List the kafka topics from the kafka container
	$(COMPOSE_COMMAND) exec kafka /opt/kafka/bin/kafka-topics.sh --list --bootstrap-server localhost:9092

.PHONY: kafka-topics-create
kafka-topics-create:  ## Create the kafka topics in KAFKA_TOPICS
	for topic in $(KAFKA_TOPICS); do \
	    $(COMPOSE_COMMAND) exec kafka /opt/kafka/bin/kafka-topics.sh --create --if-not-exists --topic $$topic --bootstrap-server localhost:9092; \
	done

.PHONY: kafka-topics-describe
kafka-topics-describe:  ## Execute kafka-topics.sh for KAFKA_TOPICS
	for topic in $(KAFKA_TOPICS); do \
	    $(COMPOSE_COMMAND) exec kafka /opt/kafka/bin/kafka-topics.sh --describe --topic $$topic --bootstrap-server localhost:9092; \
	done

# Read without committing offsets and print each message's offset, headers, and key
KAFKA_CONSUMER_PRINT_FLAGS = --command-property enable.auto.commit=false \
	--formatter-property print.offset=true \
	--formatter-property print.headers=true \
	--formatter-property print.key=true

.PHONY: kafka-topic-consume
kafka-topic-consume: KAFKA_TOPIC ?= $(firstword $(KAFKA_TOPICS))
kafka-topic-consume:  ## Print every message in KAFKA_TOPIC (default: first of KAFKA_TOPICS) with offset, headers, and key, then follow new ones until Ctrl-C, without affecting any consumer group
	@[ -n "$(KAFKA_TOPIC)" ] || { echo "error: set KAFKA_TOPIC"; exit 1; }
	@$(COMPOSE_COMMAND) exec kafka /opt/kafka/bin/kafka-console-consumer.sh \
	  --bootstrap-server localhost:9092 \
	  --topic "$(KAFKA_TOPIC)" \
	  --from-beginning \
	  $(KAFKA_CONSUMER_PRINT_FLAGS)

KAFKA_MESSAGE_FORMAT ?= json

# kcat exits non-zero if delivery fails. Its image is amd64 only, --platform avoids a warning on Apple Silicon.
# https://stackoverflow.com/questions/58716683/is-there-a-way-to-add-headers-in-kafka-console-producer-sh
# https://github.com/edenhill/kcat
# https://dev.to/de_maric/learn-how-to-use-kafkacat-the-most-versatile-kafka-cli-client-1kb4
.PHONY: kafka-produce-message
kafka-produce-message: ## Send KAFKA_MESSAGE_FILE to KAFKA_TOPIC, one message per JSON document (optional: KAFKA_MESSAGE_KEY, KAFKA_MESSAGE_HEADERS="k1:v1,k2:v2", KAFKA_MESSAGE_FORMAT=raw for one message per line)
	@[ -n "$(KAFKA_TOPIC)" ] || { echo "error: set KAFKA_TOPIC"; exit 1; }
	@[ -f "$(KAFKA_MESSAGE_FILE)" ] || { echo "error: KAFKA_MESSAGE_FILE \"$(KAFKA_MESSAGE_FILE)\" isn't a file"; exit 1; }
	@case "$(KAFKA_MESSAGE_FORMAT)" in \
	  json) messages=$$(jq -c . "$(KAFKA_MESSAGE_FILE)") || { echo "error: $(KAFKA_MESSAGE_FILE) isn't valid JSON, use KAFKA_MESSAGE_FORMAT=raw to send it as is"; exit 1; } ;; \
	  raw) messages=$$(grep -v '^$$' "$(KAFKA_MESSAGE_FILE)") ;; \
	  *) echo "error: KAFKA_MESSAGE_FORMAT must be json or raw"; exit 1 ;; \
	esac; \
	[ -n "$$messages" ] || { echo "error: $(KAFKA_MESSAGE_FILE) has no messages"; exit 1; }; \
	set -f --; \
	[ -z "$(KAFKA_MESSAGE_KEY)" ] || set -- -k "$(KAFKA_MESSAGE_KEY)"; \
	headers="$(KAFKA_MESSAGE_HEADERS)"; IFS=,; \
	for header in $$headers; do \
	  case "$$header" in *:*) ;; *) echo "error: header \"$$header\" isn't name:value"; exit 1 ;; esac; \
	  set -- "$$@" -H "$${header%%:*}=$${header#*:}"; \
	done; \
	unset IFS; \
	printf '%s\n' "$$messages" \
	  | $(DOCKER) run -i --rm --net=host --platform linux/amd64 docker.io/edenhill/kcat:1.7.1 \
	    -b localhost:9092 -t "$(KAFKA_TOPIC)" "$$@" -P \
	  || { echo "error: failed to produce to topic $(KAFKA_TOPIC), see the errors above"; exit 1; }; \
	count=$$(printf '%s\n' "$$messages" | wc -l | tr -d ' '); \
	noun=messages; [ "$$count" != 1 ] || noun=message; \
	echo "Produced $$count $$noun to topic $(KAFKA_TOPIC)"

.PHONY: kafka-group-committed
kafka-group-committed: ## Print the messages in KAFKA_TOPIC that KAFKA_GROUP_ID committed, count the ones it hasn't yet, and show which consumer owns each partition
	@[ -n "$(KAFKA_TOPIC)" ] || { echo "error: set KAFKA_TOPIC"; exit 1; }
	@[ -n "$(KAFKA_GROUP_ID)" ] || { echo "error: set KAFKA_GROUP_ID"; exit 1; }
	@partitions=$$($(COMPOSE_COMMAND) exec -T kafka /opt/kafka/bin/kafka-consumer-groups.sh \
	    --bootstrap-server localhost:9092 --describe --group "$(KAFKA_GROUP_ID)" 2>/dev/null \
	  | awk -v topic="$(KAFKA_TOPIC)" '$$2 == topic { print $$3, $$4, $$5, $$7, $$8 }'); \
	[ -n "$$partitions" ] || { echo "$(KAFKA_GROUP_ID) has no consumer and no committed offsets in $(KAFKA_TOPIC)"; exit 0; }; \
	starts=$$($(COMPOSE_COMMAND) exec -T kafka /opt/kafka/bin/kafka-get-offsets.sh \
	    --bootstrap-server localhost:9092 --topic "$(KAFKA_TOPIC)" --time -2 < /dev/null); \
	echo "$$partitions" | while read -r partition committed end consumer host; do \
	  [ -z "$$separate" ] || echo; separate=1; \
	  owner="no consumer connected"; [ "$${consumer:--}" = - ] || owner="consumer $$consumer on $$host"; \
	  case "$$committed" in \
	    ''|*[!0-9]*) echo "partition $$partition: nothing committed yet ($$owner)"; continue ;; \
	  esac; \
	  start=$$(echo "$$starts" | awk -F: -v t="$(KAFKA_TOPIC)" -v p="$$partition" '$$1 == t && $$2 == p { print $$3 }'); \
	  echo "partition $$partition: $$((committed - start)) committed, $$((end - committed)) not committed yet ($$owner)"; \
	  [ "$$committed" -gt "$$start" ] || continue; \
	  echo; \
	  $(COMPOSE_COMMAND) exec -T kafka /opt/kafka/bin/kafka-console-consumer.sh \
	    --bootstrap-server localhost:9092 \
	    --topic "$(KAFKA_TOPIC)" \
	    --partition "$$partition" \
	    --offset "$$start" \
	    --max-messages "$$((committed - start))" \
	    --timeout-ms 5000 \
	    $(KAFKA_CONSUMER_PRINT_FLAGS) \
	    < /dev/null 2>&1 | grep -Ev "KIP-848|TimeoutException|Error processing message, terminating consumer process|^Processed a total of" || true; \
	done
