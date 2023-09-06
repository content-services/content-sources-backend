BEGIN;

alter table repositories add column origin varchar default 'external' not null;
alter table repositories add column content_type varchar default 'rpm' not null;

COMMIT;
