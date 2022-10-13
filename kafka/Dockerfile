# https://kafka.apache.org/quickstart

# https://catalog.redhat.com/software/containers/ubi8/ubi/5c359854d70cc534b3a3784e
FROM registry.access.redhat.com/ubi8/ubi:8.6 AS base

ARG KAFKA_VERSION=3.3.1
ENV KAFKA_SITE_URL=https://dlcdn.apache.org/kafka/${KAFKA_VERSION}/kafka_2.13-${KAFKA_VERSION}.tgz

WORKDIR /opt
RUN curl -L "${KAFKA_SITE_URL}" \
    | tar -xzf - \
    && mv kafka_2.13-${KAFKA_VERSION} kafka

# https://catalog.redhat.com/software/containers/redhat-openjdk-18/openjdk18-openshift/58ada5701fbe981673cd6b10
# This image does not allow arm64 architecture yet
# https://catalog.redhat.com/software/containers/ubi8/openjdk-17-runtime/618bdc5f843af1624c4e4ba8
# We need an arm64 and x86_64 multiarch image
FROM registry.access.redhat.com/ubi8/openjdk-17-runtime:1.14

ENV KAFKA_HOME "/opt/kafka"

COPY --from=base /opt/kafka /opt/kafka

USER 0

RUN mkdir -p /usr/jolokia/agents \
    && curl -L "https://search.maven.org/remotecontent?filepath=org/jolokia/jolokia-jvm/1.7.1/jolokia-jvm-1.7.1.jar" > "/usr/jolokia/agents/jolokia-jvm.jar"

COPY ./kafka/scripts /opt/kafka/scripts
