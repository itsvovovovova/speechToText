package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"speechToText/src/types"
	"time"

	"golang.org/x/crypto/bcrypt"
)

type Store struct {
	db *sql.DB
}

func NewStore(db *sql.DB) *Store {
	return &Store{db: db}
}

func (s *Store) AddAuthData(username string, password string) error {
	_, err := s.db.Exec("INSERT INTO users (username, password) VALUES ($1, $2)", username, password)
	return err
}

func HashPassword(password string) (string, error) {
	bytes, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	return string(bytes), err
}

func (s *Store) CheckAuthData(username string, password string) (bool, error) {
	var hashedPassword string
	err := s.db.QueryRow("SELECT password FROM users WHERE username = $1", username).Scan(&hashedPassword)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return false, nil
		}
		return false, err
	}
	return bcrypt.CompareHashAndPassword([]byte(hashedPassword), []byte(password)) == nil, nil
}

func (s *Store) ExistUsername(username string) (bool, error) {
	var exists bool
	err := s.db.QueryRow("SELECT EXISTS(SELECT 1 FROM users WHERE username = $1)", username).Scan(&exists)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return false, nil
		}
		return false, fmt.Errorf("error checking username existence: %w", err)
	}
	return exists, nil
}

func (s *Store) DeleteTask(ctx context.Context, taskID string, username string) error {
	result, err := s.db.ExecContext(ctx, "DELETE FROM tasks WHERE task_id = $1 AND username = $2", taskID, username)
	if err != nil {
		return err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func (s *Store) GetTasksWithPagination(username string, page, pageSize int) ([]types.TaskInfo, int64, error) {
	offset := (page - 1) * pageSize

	var total int64
	if err := s.db.QueryRow("SELECT COUNT(*) FROM tasks WHERE username = $1", username).Scan(&total); err != nil {
		return nil, 0, err
	}

	rows, err := s.db.Query(`
		SELECT task_id, username, status, created_at
		FROM tasks
		WHERE username = $1
		ORDER BY created_at DESC
		LIMIT $2 OFFSET $3`,
		username, pageSize, offset,
	)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	tasks := make([]types.TaskInfo, 0)
	for rows.Next() {
		var task types.TaskInfo
		var createdAt time.Time
		if err := rows.Scan(&task.TaskID, &task.Username, &task.Status, &createdAt); err != nil {
			return nil, 0, err
		}
		task.Created = createdAt.Format(time.RFC3339)
		tasks = append(tasks, task)
	}
	return tasks, total, rows.Err()
}

func (s *Store) ReadTask(ctx context.Context, taskID, owner string) (string, *string, error) {
	var status string
	var result *string
	err := s.db.QueryRowContext(ctx, "SELECT status, result FROM tasks WHERE task_id = $1 AND username = $2", taskID, owner).Scan(&status, &result)
	return status, result, err
}
