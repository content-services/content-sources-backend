BEGIN;

INSERT INTO domains (org_id, domain_name)
    VALUES ('-4', 'public-lightwell-demo')
    ON CONFLICT DO NOTHING;
	
COMMIT;
