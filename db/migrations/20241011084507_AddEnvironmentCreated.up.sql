BEGIN;

alter table templates add column rhsm_environment_created boolean NOT NULL DEFAULT false;
update templates set rhsm_environment_created = true
                 where templates.uuid in (select object_uuid from tasks where type = 'update-template-content' and status = 'completed');

COMMIT;
