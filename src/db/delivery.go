package db

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/google/uuid"
)

var ErrIdempotencyConflict = errors.New("idempotency key already used for different audio")

func (s *Store) Enqueue(ctx context.Context, id, owner, audio, key string) (string, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return "", err
	}
	defer tx.Rollback()

	var inserted string
	err = tx.QueryRowContext(ctx, `
		INSERT INTO tasks (task_id, username, audio, status, idempotency_key)
		VALUES ($1, $2, $3, 'queued', NULLIF($4, ''))
		ON CONFLICT (username, idempotency_key) DO NOTHING
		RETURNING task_id`, id, owner, audio, key).Scan(&inserted)
	if errors.Is(err, sql.ErrNoRows) {
		var existingAudio string
		err = tx.QueryRowContext(ctx, `
			SELECT task_id, audio FROM tasks WHERE username = $1 AND idempotency_key = $2
		`, owner, key).Scan(&id, &existingAudio)
		if err != nil {
			return "", err
		}
		if existingAudio != audio {
			return "", ErrIdempotencyConflict
		}
		return id, tx.Commit()
	}
	if err != nil {
		return "", err
	}
	if _, err = tx.ExecContext(ctx, "INSERT INTO task_outbox (task_id) VALUES ($1)", inserted); err != nil {
		return "", err
	}
	return inserted, tx.Commit()
}

func (s *Store) PublishNext(ctx context.Context, publish func(context.Context, string, int) error) (bool, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return false, err
	}
	defer tx.Rollback()

	var id string
	var attempt int
	err = tx.QueryRowContext(ctx, `
		SELECT o.task_id, t.attempts + 1 FROM task_outbox o JOIN tasks t ON t.task_id = o.task_id
		WHERE o.created_at <= now() ORDER BY o.created_at, o.task_id LIMIT 1 FOR UPDATE OF o SKIP LOCKED
	`).Scan(&id, &attempt)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if err = publish(ctx, id, attempt); err != nil {
		return false, err
	}
	if _, err = tx.ExecContext(ctx, "DELETE FROM task_outbox WHERE task_id = $1", id); err != nil {
		return false, err
	}
	return true, tx.Commit()
}

type AttemptOutcome struct {
	Status string
	Age    time.Duration
}

// ProcessTask claims the current attempt with a bounded lease. A redelivery
// can reclaim it after expiry. A superseded worker cannot commit a result.
func (s *Store) ProcessTask(ctx context.Context, id string, attempt int, transcribe func(string) (string, bool, error)) (AttemptOutcome, error) {
	token := uuid.NewString()
	var audio string
	var createdAt sql.NullTime
	err := s.db.QueryRowContext(ctx, `
		UPDATE tasks SET status = 'processing', lease_token = $3, lease_until = now() + interval '45 seconds'
		WHERE task_id = $1 AND attempts = $2 - 1
		AND (status = 'queued' OR (status = 'processing' AND lease_until <= now()))
		AND NOT EXISTS (SELECT 1 FROM task_outbox WHERE task_id = $1)
		RETURNING audio, created_at
	`, id, attempt, token).Scan(&audio, &createdAt)
	if errors.Is(err, sql.ErrNoRows) {
		var pending bool
		err = s.db.QueryRowContext(ctx, `
			SELECT EXISTS (
				SELECT 1 FROM tasks WHERE task_id = $1 AND attempts = $2 - 1
				AND status IN ('queued', 'processing')
			)
		`, id, attempt).Scan(&pending)
		if err == nil && pending {
			return AttemptOutcome{}, ErrDeliveryPending
		}
		return AttemptOutcome{}, err
	}
	if err != nil {
		return AttemptOutcome{}, err
	}

	result, retry, providerErr := transcribe(audio)
	status := "completed"
	var storedResult *string = &result
	if providerErr != nil {
		storedResult = nil
		status = "failed"
		if retry && attempt < 5 {
			status = "queued"
		}
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return AttemptOutcome{}, err
	}
	defer tx.Rollback()
	res, err := tx.ExecContext(ctx, `
		UPDATE tasks SET status = $3, result = $4, attempts = $5, lease_token = NULL, lease_until = NULL
		WHERE task_id = $1 AND lease_token = $2 AND lease_until > now()
	`, id, token, status, storedResult, attempt)
	if err != nil {
		return AttemptOutcome{}, err
	}
	rows, err := res.RowsAffected()
	if err != nil {
		return AttemptOutcome{}, err
	}
	if rows == 0 {
		return AttemptOutcome{}, ErrDeliveryPending
	}
	if status == "queued" {
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO task_outbox (task_id, created_at)
			VALUES ($1, now() + $2 * interval '1 second')
		`, id, 1<<attempt); err != nil {
			return AttemptOutcome{}, err
		}
	}
	if err := tx.Commit(); err != nil {
		return AttemptOutcome{}, err
	}
	outcome := AttemptOutcome{Status: status}
	if createdAt.Valid {
		outcome.Age = time.Since(createdAt.Time)
	}
	return outcome, nil
}

var ErrDeliveryPending = errors.New("delivery pending publication or held by another worker")
