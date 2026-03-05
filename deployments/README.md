# `/deployments`

IaaS, PaaS, system and container orchestration deployment configurations and templates (docker-compose, kubernetes/helm, mesos, terraform, bosh).

This directory includes a template system for generating `deployment.yaml` from a template and grouped environment variables, reducing duplication and keeping env vars in one place.

## Template system

- **`build/deployment.template.yaml`** – OpenShift/ClowdApp template with placeholders for env blocks.
- **`build/env-variables.yaml`** – A single list under the key `env`. Each entry has `name` and `value` or `valueFrom`. Every deployment and job gets this full list.
- **`build/process-template.py`** – Script that merges the template and env-variables to produce `deployment.yaml`.
- **`deployment.yaml`** – Generated file (do not edit by hand; regenerate with `make deployment-generate`).

### Generate deployment.yaml

```bash
make deployment-generate
```

This writes `deployments/deployment.yaml` from the template and env-variables. CI runs this and fails if the result differs from the committed file.

### Other targets

```bash
make deployment-clean    # Remove generated deployment.yaml
make deployment-validate # Validate with OpenShift CLI (oc process --local)
```

### Adding new environment variables

Edit `build/env-variables.yaml` and add a new item to the `env` list with `name` and `value` or `valueFrom`. It will be included for every deployment and job.

