# Deployment Template System

This directory contains a template system for generating the `deployment.yaml` file from a template and organized environment variables. This system helps reduce duplication and makes it easier to manage environment variables across different deployments and jobs.

## Files

- `build/deployment.template.yaml` - The base template file with placeholders for environment variables
- `build/env-variables.yaml` - Organized environment variables grouped by usage
- `build/process-template.py` - Python script that combines the template and environment variables
- `deployment.yaml` - Generated deployment file (created by the template processor)

## Environment Variables Organization

The environment variables in `env-variables.yaml` are organized into the following groups:

### Common Variables (`common`)
Variables used across all deployments and jobs:
- Core application settings (CLOWDER_ENABLED, LOGGING_LEVEL, etc.)
- Certificates and secrets (RH_CDN_CERT_PAIR, SENTRY_DSN)
- Pulp client settings (CLIENTS_PULP_*)
- RBAC settings (CLIENTS_RBAC_BASE_URL)
- Feature flags (FEATURES_*)
- Candlepin settings (CLIENTS_CANDLEPIN_*)
- Feature service settings (CLIENTS_FEATURE_SERVICE_*)
- Roadmap settings (CLIENTS_ROADMAP_SERVER)

### Service-Specific Variables (`service_specific`)
Variables used only by the API service:
- Repository import filter (OPTIONS_REPOSITORY_IMPORT_FILTER)
- Pulp database settings (CLIENTS_PULP_DATABASE_*)

### Job-Specific Variables (`job_specific`)
Variables used by specific cron jobs:

#### `process_repos`
- Snapshot retention settings (OPTIONS_SNAPSHOT_RETAIN_DAYS_LIMIT)

#### `transform_pulp_logs`
- CloudWatch settings (CLIENTS_PULP_LOG_PARSER_CLOUDWATCH_*)
- S3 settings (CLIENTS_PULP_LOG_PARSER_S3_*)

## Template Placeholders

The template file uses the following placeholders that get replaced with environment variables:

- `{{ENV_COMMON}}` - Common environment variables
- `{{ENV_SERVICE_SPECIFIC}}` - Service-specific environment variables
- `{{ENV_JOB_PROCESS_REPOS}}` - Process repos job-specific variables
- `{{ENV_JOB_TRANSFORM_PULP_LOGS}}` - Transform pulp logs job-specific variables

## Usage

### Generate deployment.yaml
```bash
make deployment-generate
```

This command:
1. Reads the `deployment.template.yaml` file
2. Reads the `env-variables.yaml` file
3. Combines them to generate the final `deployment.yaml` file

### Clean up generated files
```bash
make deployment-clean
```

This removes the generated `deployment.yaml` file.

### Validate the generated template
```bash
make deployment-validate
```

This validates the generated deployment template (requires OpenShift CLI).

### Show help
```bash
make deployment-help
```

This shows all available deployment-related make targets.

## Adding New Environment Variables

To add new environment variables:

1. **Common variables**: Add them to the `common` section in `env-variables.yaml`
2. **Service-specific variables**: Add them to the `service_specific` section
3. **Job-specific variables**: Add them to the appropriate job section in `job_specific`

### Example: Adding a new common variable

```yaml
common:
  # ... existing variables ...
  - name: NEW_VARIABLE
    value: ${NEW_VARIABLE}
```

### Example: Adding a new job-specific variable

```yaml
job_specific:
  # ... existing jobs ...
  new_job:
    - name: NEW_JOB_VARIABLE
      value: ${NEW_JOB_VARIABLE}
```

Then add the placeholder `{{ENV_JOB_NEW_JOB}}` to the template and update the Python script to handle it.

## File Structure

```
deployments/
├── README.md                    # This documentation
├── deployment.yaml              # Generated deployment file
└── build/                       # Build artifacts and source files
    ├── deployment.template.yaml # Template file
    ├── env-variables.yaml       # Environment variables
    └── process-template.py      # Template processor script
```

## Benefits

1. **Reduced duplication**: Environment variables are defined once and reused
2. **Better organization**: Variables are grouped by their usage
3. **Easier maintenance**: Changes to common variables only need to be made in one place
4. **Template flexibility**: Easy to add new job types or modify existing ones
5. **Validation**: The generated YAML can be validated for syntax correctness

## Migration from Original deployment.yaml

The original `deployment.yaml` file has been replaced with this template system. The generated file should be functionally equivalent to the original, but with better organization and reduced duplication.
