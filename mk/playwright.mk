
playwright: ## Runs playwright api tests
	chmod +x scripts/create_env_if_not_exist.sh \
	  && scripts/create_env_if_not_exist.sh \
	  && cd _playwright-tests \
	  && yarn install \
	  && yarn playwright test