BEGIN;

-- A staged row was never claimable. Mark it failed rather than deleting an
-- upload record while rolling back the status constraint.
UPDATE bundle_action_files
SET status = 'failed',
    error_code = COALESCE(error_code, 'ACTION_FILE_STAGING_ROLLBACK'),
    error_message = COALESCE(error_message, 'Upload staging was interrupted during schema rollback'),
    updated_at = CURRENT_TIMESTAMP
WHERE status = 'staging';

ALTER TABLE bundle_action_files
    DROP CONSTRAINT IF EXISTS bundle_action_files_status_valid;

ALTER TABLE bundle_action_files
    ADD CONSTRAINT bundle_action_files_status_valid
    CHECK (status IN ('pending','processing','processed','failed','delete_pending','deleted'));

COMMIT;
