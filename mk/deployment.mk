# Deployment-related targets

.PHONY: deployment-generate deployment-clean deployment-validate deployment-diff deployment-help

deployment-generate: ## Generate deployment.yaml from template and environment variables
	@echo "Generating deployment.yaml from template and environment variables..."
	@python3 deployments/build/process-template.py
	@echo "Deployment file generated: deployments/deployment.yaml"

deployment-validate: deployment-generate ## Validate the generated deployment template
	@echo "Validating generated deployment template..."
	@oc process --local -f deployments/deployment.yaml > /dev/null
	@echo "Deployment template validation successful"
