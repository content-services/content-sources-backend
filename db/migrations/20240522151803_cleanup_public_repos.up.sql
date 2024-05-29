BEGIN;
update repositories set public = false where public = true and url NOT LIKE '%/';
COMMIT;
