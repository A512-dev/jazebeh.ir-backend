BEGIN;

ALTER TABLE bundle_action_files
    DROP CONSTRAINT IF EXISTS bundle_action_files_status_valid;

ALTER TABLE bundle_action_files
    ADD CONSTRAINT bundle_action_files_status_valid
    CHECK (status IN ('staging','pending','processing','processed','failed','delete_pending','deleted'));

COMMIT;
