BEGIN;

-- Remove the dummy Beacon vulnerabilities so the seed can insert them again,
-- including a beacon sync timestamp. This runs in every environment.
--
-- A vulnerability is deleted only when both are true:
--   * its uuid is in the fixed seed prefix 00000000-0000-4000-8000-
--   * its vulnerability_key uses the seed prefix PROJ-
-- All 60 rows in db/seeds/lightwell_vulnerabilities.sql match both.
-- Synced Beacon issues use uuid.New() and a Jira issue key, so they do not match.
-- Customer and ticket rows are deleted only for those vulnerabilities.
-- lightwell_beacon_sync is not modified, so a real sync timestamp is kept.

DELETE FROM lightwell_vulnerability_support_tickets AS ticket
USING lightwell_vulnerabilities AS vulnerability
WHERE ticket.vulnerability_uuid = vulnerability.uuid
    AND vulnerability.uuid::text LIKE '00000000-0000-4000-8000-%'
    AND vulnerability.vulnerability_key LIKE 'PROJ-%';

DELETE FROM lightwell_vulnerability_customers AS customer
USING lightwell_vulnerabilities AS vulnerability
WHERE customer.vulnerability_uuid = vulnerability.uuid
    AND vulnerability.uuid::text LIKE '00000000-0000-4000-8000-%'
    AND vulnerability.vulnerability_key LIKE 'PROJ-%';

DELETE FROM lightwell_vulnerabilities
WHERE uuid::text LIKE '00000000-0000-4000-8000-%'
    AND vulnerability_key LIKE 'PROJ-%';

COMMIT;
