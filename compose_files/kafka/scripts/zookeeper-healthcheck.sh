#!/bin/bash

	# --health-cmd '/bin/bash -c "[ \"` curl -o /dev/null -s -w \"%{http_code}\n\" http://localhost:8778/jolokia/ `\" == \"200\" ]"' \

function zookeeper-get-version {
    "${KAFKA_HOME}/bin/zookeeper-shell.sh" localhost:2181 << "EOF"
version
quit
EOF
}

function zookeeper-check-jolokia {
    [ "$( curl -o /dev/null -s -w "%{http_code}\n" http://kafka:8778/jolokia/ )" == "200" ]
}

# TODO I am not sure this is the right way for the healtch-check
zookeeper-check-jolokia

