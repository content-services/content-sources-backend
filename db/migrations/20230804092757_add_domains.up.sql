BEGIN;

CREATE TABLE IF NOT EXISTS domains (
 org_id VARCHAR (255) NOT NULL,
 domain_name VARCHAR (255) NOT NULL
);


ALTER TABLE domains
ADD CONSTRAINT domains_org_id_unique UNIQUE (org_id);

ALTER TABLE domains
ADD CONSTRAINT domains_name_unique UNIQUE (domain_name);

COMMIT;
