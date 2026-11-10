BEGIN;

-- The retired lightwell/npm/* repositories are not restored. They are rebuilt
-- from pkg/external_repos/lightwell_repos.json by the repository import, which
-- now publishes them under lightwell/javascript/*.

COMMIT;
