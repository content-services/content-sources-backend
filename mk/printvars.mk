##
# This file only contain the helper rule which print the
# variable values (without expansion), so that we can see
# what is the definition before expansion.
##
.PHONY: printvars
printvars: ## Print variable name and values
	@$(foreach V, $(sort $(.VARIABLES)),$(if $(filter-out environment% default automatic,$(origin $V)),$(info $V=$(value $V))))
