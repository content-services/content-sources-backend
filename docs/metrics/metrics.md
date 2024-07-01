# Metrics

Our app uses [Prometheus](https://prometheus.io/) to continuously monitor performance and alert if performance in an area has fallen too low.

Prometheus requires our app to return metrics on the `/metrics` endpoint. A metric is a point of data about our app. These are metrics we define, implement and eventually use to configure Prometheus to alert on. Prometheus queries the metrics endpoint every 15 seconds (or as defined in the configuration) to get the latest information.

All of our metrics are defined in the instrumentation package.
https://github.com/content-services/content-sources-backend/blob/173f764d031da46665136a317caa8213e3677ad7/pkg/instrumentation/metrics.go#L12-L28

## How are metrics implemented?

One example of a metric is `repository_configs_total`, which records the total number of repository configurations across the app.

The metric is registered in `metrics.go`

https://github.com/content-services/content-sources-backend/blob/173f764d031da46665136a317caa8213e3677ad7/pkg/instrumentation/metrics.go#L87-L91

Every 15 seconds when Prometheus scrapes the app, the collector is iterated and a query is run to get the total number of repository configurations. All of the collectors are defined in `collector.go`. For each metric, the collector calls a method that returns the metric value.

https://github.com/content-services/content-sources-backend/blob/173f764d031da46665136a317caa8213e3677ad7/pkg/instrumentation/custom/collector.go#L63