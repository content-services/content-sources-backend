##
# Rules for generating and validating deployment.yaml from template and env-variables.
##

.PHONY: deployment-generate deployment-validate

deployment-generate: ## Generate deployment.yaml from deployment.template.yaml and env-variables.yaml
	@python3 deployments/build/process-template.py

deployment-validate: deployment-generate ## Validate generated deployment template (requires OpenShift CLI)
	@oc process --local -f deployments/deployment.yaml > /dev/null && echo "Deployment template validation successful"
