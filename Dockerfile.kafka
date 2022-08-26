# https://catalog.redhat.com/software/containers/redhat-openjdk-18/openjdk18-openshift/58ada5701fbe981673cd6b10
FROM registry.access.redhat.com/redhat-openjdk-18/openjdk18-openshift:1.12-1.1658775370

WORKDIR /opt
RUN curl "https://dlcdn.apache.org/kafka/3.2.1/kafka_2.13-3.2.1.tgz" \
    | tar -xzf - \
    && ln -svf kafka_2.13-3.2.1 kafka

RUN mkdir -p /usr/jolokia/agents \
    && curl -L "https://search.maven.org/remotecontent?filepath=org/jolokia/jolokia-jvm/1.7.1/jolokia-jvm-1.7.1.jar" > "/usr/jolokia/agents/jolokia-jvm.jar"
