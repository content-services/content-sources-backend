BEGIN;

alter table tasks
    add column
    if not exists
        try_cancel bool
        default false;

COMMIT;
