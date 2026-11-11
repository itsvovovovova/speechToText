DROP TABLE task_outbox;
ALTER TABLE tasks DROP COLUMN attempts;
ALTER TABLE tasks DROP COLUMN lease_token, DROP COLUMN lease_until;
UPDATE tasks SET status = 'in progress' WHERE status IN ('queued', 'processing');
ALTER TABLE tasks DROP COLUMN idempotency_key;
