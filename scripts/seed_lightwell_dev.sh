#!/usr/bin/env bash
# Load local Lightwell vulnerability, advisory, and package seed data.
# The Java and Python repositories must already exist. scripts/create_lightwell_repo.sh
# creates them and then calls this script.
# Re-running skips the vulnerability inserts when those rows are already present
# and refreshes advisories and packages.
set -euo pipefail

root="$(cd "$(dirname "$0")/.." && pwd)"
conn="${LIGHTWELL_SEED_PSQL:-sslmode=disable dbname=content user=content host=localhost port=5433 password=content}"

psql_file() {
    psql "$conn" -v ON_ERROR_STOP=1 -f "$root/$1"
}

seeded="$(psql "$conn" -v ON_ERROR_STOP=1 -tA -c "SELECT 1 FROM lightwell_vulnerabilities WHERE uuid = '00000000-0000-4000-8000-000000000001'")"
if [[ "$seeded" == "1" ]]; then
    echo "==> Lightwell vulnerability seed already present; refreshing advisories and packages."
else
    psql_file db/seeds/lightwell_vulnerabilities.sql
fi

psql_file db/seeds/lightwell_advisories.sql
psql_file db/seeds/lightwell_packages.sql
