# Deployment-related targets

.PHONY: deployment-generate deployment-clean deployment-validate deployment-diff deployment-help

# Generate deployment.yaml from template and environment variables
deployment-generate:
	@echo "Generating deployment.yaml from template and environment variables..."
	@python3 deployments/build/process-template.py
	@echo "Deployment file generated: deployments/deployment.yaml"

# Clean up generated files
deployment-clean:
	@echo "Cleaning up generated deployment files..."
	@rm -f deployments/deployment.yaml

# Validate the generated deployment template
deployment-validate: deployment-generate
	@echo "Validating generated deployment template..."
	@oc process --local -f deployments/deployment.yaml > /dev/null
	@echo "Deployment template validation successful"

# Show differences between template and generated file
deployment-diff: deployment-generate
	@echo "Showing differences between template and generated file..."
	@if [ -f deployments/deployment.yaml ]; then \
		echo "Generated deployment.yaml exists"; \
	else \
		echo "Generated deployment.yaml does not exist"; \
	fi

# Help target
deployment-help:
	@echo "Available deployment targets:"
	@echo "  deployment-generate     - Generate deployment.yaml from template and env vars"
	@echo "  deployment-clean        - Clean up generated files"
	@echo "  deployment-validate     - Validate the generated deployment template"
	@echo "  deployment-diff         - Show status of generated file"
	@echo "  deployment-help         - Show this help message" 