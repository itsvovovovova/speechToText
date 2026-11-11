ALTER TABLE tasks ADD COLUMN idempotency_key TEXT;
ALTER TABLE tasks ADD COLUMN attempts INTEGER NOT NULL DEFAULT 0;
ALTER TABLE tasks ADD COLUMN lease_token TEXT, ADD COLUMN lease_until TIMESTAMPTZ;
ALTER TABLE tasks ADD UNIQUE (username, idempotency_key);

CREATE TABLE task_outbox (
    task_id VARCHAR(1000) PRIMARY KEY REFERENCES tasks(task_id) ON DELETE CASCADE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

UPDATE tasks SET status = 'queued' WHERE status = 'in progress';
INSERT INTO task_outbox (task_id) SELECT task_id FROM tasks WHERE status = 'queued';
