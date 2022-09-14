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
[ "${KAFKA_ADVERTISED_LISTENERS}" != "" ] || KAFKA_ADVERTISED_LISTENERS="PLAINTEXT://kafka:9092"
[ "${ZOOKEEPER_CLIENT_PORT}" != "" ] || ZOOKEEPER_CLIENT_PORT="2191"
[ "${KAFKA_LISTENER_SECURITY_PROTOCOL_MAP}" != "" ] || KAFKA_LISTENER_SECURITY_PROTOCOL_MAP="PLAINTEXT:PLAINTEXT"
[ "${KAFKA_INTER_BROKER_LISTENER_NAME}" != "" ] || KAFKA_INTER_BROKER_LISTENER_NAME="PLAINTEXT"
[ "${KAFKA_OPTS}" != "" ] || KAFKA_OPTS="-javaagent:/usr/jolokia/agents/jolokia-jvm.jar=host=0.0.0.0"
[ "${KAFKA_TOPIC}" != "" ] || KAFKA_TOPIC="repos.created"

# TODO Handle the variables to generate the configuration or execute the necessary commands
#      to get the container running as we need

function create_topics {
    while ! /opt/kafka/bin/kafka-topics.sh --zookeeper localhost:2181/kafka --create --topic "${KAFKA_TOPIC}" --replication-factor 1 --partitions 3; do
        sleep 1
    done
    /opt/kafka/bin/kafka-topics.sh --zookeeper localhost:2181/kafka --topic "${KAFKA_TOPIC}" --describe
}

nohup create_topics &

exec "${KAFKA_HOME}/bin/kafka-server-start.sh" /tmp/config/server.properties # "${KAFKA_OPTS}"
