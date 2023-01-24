#!/bin/bash -xv

# Environment variables:
#
#   ZOOKEEPER_CLIENT_PORT
#   ZOOKEEPER_OPTS
#
exec "${KAFKA_HOME}/bin/zookeeper-server-start.sh" /tmp/config/zookeeper.properties # "${ZOOKEEPER_OPTS}"
