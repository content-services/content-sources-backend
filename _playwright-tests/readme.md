# Use the correct node version

Ensure the correct [node version](_playwright-tests/.nvmrc), is installed and in use:  
 `cd _playwright-tests` and `nvm use`

# Install NPM packages

`yarn install`

# Install Playwright browsers and dependencies

`yarn playwright install `

OR

If using any os other than Fedora/Rhel (IE: mac, ubuntu linux):

`yarn playwright install  --with-deps`

# Make sure you have your .env set

Copy the [example env](_playwright-tests/example.env) file and create a file at: \_playwright-tests/.env.

For local development only the BASE_URL: `http://127.0.0.1:8000` is required, which is already set in the example config.

# Run your tests:

Ensure that the backend server is running

`yarn playwright test`

# Run a single test:

`yarn playwright test UI/CreateCustomRepo.spec.ts`
