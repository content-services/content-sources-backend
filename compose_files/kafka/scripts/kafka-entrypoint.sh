#!/bin/bash -xv

# Environment variables:
#
#   KAFKA_BROKER_ID=1
#   KAFKA_ZOOKEEPER_CONNECT="zookeeper:2181"
#   KAFKA_ADVERTISED_LISTENERS="PLAINTEXT://kafka:9092"
#   ZOOKEEPER_CLIENT_PORT=$(ZOOKEEPER_CLIENT_PORT)
#   KAFKA_LISTENER_SECURITY_PROTOCOL_MAP=PLAINTEXT:PLAINTEXT
#   KAFKA_INTER_BROKER_LISTENER_NAME=PLAINTEXT
#   KAFKA_OPTS='-javaagent:/usr/jolokia/agents/jolokia-jvm.jar=host=0.0.0.0'
#
[ "${KAFKA_BROKER_ID}" != "" ] || KAFKA_BROKER_ID="1"
[ "${KAFKA_ZOOKEEPER_CONNECT}" != "" ] || KAFKA_ZOOKEEPER_CONNECT="zookeeper:2181"
[ "${KAFKA_ADVERTISED_LISTENERS}" != "" ] || KAFKA_ADVERTISED_LISTENERS="PLAINTEXT://localhost:9092"
[ "${ZOOKEEPER_CLIENT_PORT}" != "" ] || ZOOKEEPER_CLIENT_PORT="2191"
[ "${KAFKA_LISTENER_SECURITY_PROTOCOL_MAP}" != "" ] || KAFKA_LISTENER_SECURITY_PROTOCOL_MAP="PLAINTEXT:PLAINTEXT"
[ "${KAFKA_INTER_BROKER_LISTENER_NAME}" != "" ] || KAFKA_INTER_BROKER_LISTENER_NAME="PLAINTEXT"
[ "${KAFKA_OPTS}" != "" ] || KAFKA_OPTS="-javaagent:/usr/jolokia/agents/jolokia-jvm.jar=host=0.0.0.0"
[ "${KAFKA_TOPICS}" != "" ] || KAFKA_TOPICS="repos.created"

function create_topics {
    sleep 2
    for item in ${KAFKA_TOPICS}
    do
        while ! "${KAFKA_HOME}/bin/kafka-topics.sh" --topic "${item}" --bootstrap-server localhost:9092 --describe; do
            "${KAFKA_HOME}/bin/kafka-topics.sh" --create --bootstrap-server localhost:9092 --topic "${item}"
            sleep 1
        done
    done
    "${KAFKA_HOME}/bin/kafka-topics.sh" --bootstrap-server localhost:9092 --list
    for item in ${KAFKA_TOPICS}
    do
        "${KAFKA_HOME}/bin/kafka-topics.sh" --topic "${item}" --bootstrap-server localhost:9092 --describe
    done
    exec 2>/dev/null 1>/dev/null
    while true; do sleep 5; done
}

create_topics &

exec "${KAFKA_HOME}/bin/kafka-server-start.sh" /tmp/config/server.properties # "${KAFKA_OPTS}"
