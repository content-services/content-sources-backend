##
# This makefile provide rules to start / stop a
# local prometheus service.
#
# Variables:
#   PROMETHEUS_VERSION
#   PROMETHEYS_CONFIG
#
# See the container tags into the link below:
#   https://hub.docker.com/r/prom/prometheus/tags
#
# See also the prometheus documentation at:
#   https://prometheus.io/docs/introduction/overview/
##

PROMETHEUS_VERSION ?= v2.40.2
PROMETHEYS_CONFIG ?= $(PROJECT_DIR)/configs/prometheus.yaml
PROMETHEUS_UI_PORT ?= 9090

.PHONY: prometheus-up
prometheus-up: ## Start prometheus service (local access at http://localhost:9090)
	$(DOCKER) run --rm -d \
	  --name prometheus \
	  -p "$(PROMETHEUS_UI_PORT):9090" \
	  --volume "$(PROMETHEYS_CONFIG):/etc/prometheus/prometheus.yml:ro,z" \
	  docker.io/prom/prometheus:$(PROMETHEUS_VERSION)

.PHONY: prometheus-down
prometheus-down:  ## Stop prometheus service
	! $(DOCKER) container inspect prometheus &> /dev/null || $(DOCKER) container stop prometheus

.PHONY: prometheus-clean
prometheus-clean: prometheus-down  ## Clean the prometheus instance
	! $(DOCKER) container inspect prometheus &> /dev/null || $(DOCKER) container rm prometheus

.PHONY: prometheus-logs
prometheus-logs: ## Tail prometheus logs
	$(DOCKER) container logs --tail 10 -f prometheus

.PHONY: prometheus-ui
prometheus-ui:  ## Open browser with the prometheus ui
	xdg-open http://localhost:9090