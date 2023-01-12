# https://kafka.apache.org/quickstart

# https://catalog.redhat.com/software/containers/ubi8/ubi/5c359854d70cc534b3a3784e
FROM quay.io/centos/centos:stream9 AS base

COPY ./compose_files/kafka/scripts/kafka-print-last-version.py /kafka-print-last-version.py

WORKDIR /opt
RUN dnf install -y python3-requests
RUN /kafka-print-last-version.py > kafka-version

RUN curl -L "https://dlcdn.apache.org/kafka/$(cat kafka-version)/kafka_2.13-$(cat kafka-version).tgz" \
    | tar -xzf - \
    && mv kafka_2.13-$(cat kafka-version) kafka

# https://catalog.redhat.com/software/containers/redhat-openjdk-18/openjdk18-openshift/58ada5701fbe981673cd6b10
# This image does not allow arm64 architecture yet
# https://catalog.redhat.com/software/containers/ubi8/openjdk-17-runtime/618bdc5f843af1624c4e4ba8
# We need an arm64 and x86_64 multiarch image
FROM quay.io/centos/centos:stream
RUN dnf install -y java-17-openjdk-headless

ENV KAFKA_HOME "/opt/kafka"
COPY --from=base /opt/kafka /opt/kafka

USER 0

RUN mkdir -p /usr/jolokia/agents \
    && curl -L "https://search.maven.org/remotecontent?filepath=org/jolokia/jolokia-jvm/1.7.1/jolokia-jvm-1.7.1.jar" > "/usr/jolokia/agents/jolokia-jvm.jar"

COPY ./compose_files/kafka/scripts /opt/kafka/scripts
