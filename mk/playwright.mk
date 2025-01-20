
playwright: ## Runs playwright api tests
	cd _playwright-tests \
	  && yarn install \
	  && yarn playwright test