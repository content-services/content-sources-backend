##
# This file adds rules to generate code from the
# event message specification
#
# This is based on the repository below:
# https://github.com/RedHatInsights/playbook-dispatcher
#
# https://github.com/atombender/go-jsonschema
##

GOJSONSCHEMA := $(GO_OUTPUT)/gojsonschema
# see: https://github.com/atombender/go-jsonschema/issues/32
# v0.9.0 breaks on 'additionalProperties'
GOJSONSCHEMA_VERSION := v0.8.0

EVENT_SCHEMA_DIR := $(PROJECT_DIR)/pkg/event/schema
EVENT_MESSAGE_DIR := $(PROJECT_DIR)/pkg/event/message

SCHEMA_YAML_FILES := $(wildcard $(EVENT_SCHEMA_DIR)/*.yaml)
SCHEMA_JSON_FILES := $(patsubst $(EVENT_SCHEMA_DIR)/%.yaml,$(EVENT_SCHEMA_DIR)/%.json,$(wildcard $(EVENT_SCHEMA_DIR)/*.yaml))

.PHONY: gen-event-messages
gen-event-messages: $(GOJSONSCHEMA) $(SCHEMA_JSON_FILES)  ## Generate event messages from schemas
	@[ -e "$(EVENT_MESSAGE_DIR)" ] || mkdir -p "$(EVENT_MESSAGE_DIR)"
	$(GOJSONSCHEMA) -p message "$(EVENT_SCHEMA_DIR)/introspectRequest.message.json" -o "$(EVENT_MESSAGE_DIR)/introspect_request.types.gen.go"

$(EVENT_SCHEMA_DIR)/%.json: $(EVENT_SCHEMA_DIR)/%.yaml
	@[ -e "$(EVENT_MESSAGE_DIR)" ] || mkdir -p "$(EVENT_MESSAGE_DIR)"
	yaml2json "$<" "$@"

.PHONY: install-gojsonschema
install-gojsonschema: $(GOJSONSCHEMA)

# go install github.com/atombender/go-jsonschema/cmd/gojsonschema
$(GOJSONSCHEMA):
	@{\
		export GOPATH="$(shell mktemp -d "$(PROJECT_DIR)/tmp.XXXXXXXX" 2>/dev/null)" ; \
		echo "Using GOPATH='$${GOPATH}'" ; \
		[ "$${GOPATH}" != "" ] || { echo "error:GOPATH is empty"; exit 1; } ; \
		export GOBIN="$(dir $(GOJSONSCHEMA))" ; \
		echo "Installing 'gojsonschema' at '$(GOJSONSCHEMA)'" ; \
		go install github.com/atombender/go-jsonschema/cmd/gojsonschema@$(GOJSONSCHEMA_VERSION) ; \
		find "$${GOPATH}" -type d -exec chmod u+w {} \; ; \
		rm -rf "$${GOPATH}" ; \
	}
