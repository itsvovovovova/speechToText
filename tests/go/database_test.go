package main

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"speechToText/src/db"
)

func TestGetTasksWithPagination(t *testing.T) {
	if testStore == nil {
		t.Skip("DB not available")
	}
	tests := []struct {
		name      string
		username  string
		page      int
		pageSize  int
		expectErr bool
	}{
		{name: "Valid pagination", username: "testuser", page: 1, pageSize: 10},
		{name: "Invalid page (zero)", username: "testuser", page: 0, pageSize: 10, expectErr: true},
		{name: "Invalid page size (zero)", username: "testuser", page: 1, pageSize: 0},
		{name: "Empty username", username: "", page: 1, pageSize: 10},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tasks, total, err := testStore.GetTasksWithPagination(tt.username, tt.page, tt.pageSize)
			if tt.expectErr && err == nil {
				t.Errorf("Expected error but got none")
			}
			if !tt.expectErr && err != nil {
				t.Errorf("Unexpected error: %v", err)
			}
			if !tt.expectErr && tasks == nil {
				t.Errorf("Tasks should not be nil")
			}
			if total < 0 {
				t.Errorf("Total should not be negative")
			}
		})
	}
}

func TestDeleteTask(t *testing.T) {
	if testStore == nil {
		t.Skip("DB not available")
	}
	tests := []struct {
		name      string
		taskID    string
		username  string
		expectErr bool
	}{
		{name: "Non-existent task", taskID: "non-existent-id", username: "testuser", expectErr: true},
		{name: "Empty task ID", taskID: "", username: "testuser", expectErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := testStore.DeleteTask(context.Background(), tt.taskID, tt.username)
			if tt.expectErr && err == nil {
				t.Errorf("Expected error but got none")
			}
			if !tt.expectErr && err != nil {
				t.Errorf("Unexpected error: %v", err)
			}
		})
	}
}

func seedUser(t *testing.T, username, password string) {
	t.Helper()
	hash, err := db.HashPassword(password)
	if err != nil {
		t.Fatal(err)
	}
	if err := testStore.AddAuthData(username, hash); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = testSQL.Exec("DELETE FROM users WHERE username = $1", username) })
}

func TestUserExists(t *testing.T) {
	if testStore == nil {
		t.Skip("DB not available")
	}
	username := uuid.NewString()
	seedUser(t, username, "test-password")
	for candidate, want := range map[string]bool{username: true, uuid.NewString(): false, "": false} {
		got, err := testStore.ExistUsername(candidate)
		if err != nil || got != want {
			t.Errorf("ExistUsername(%q) = %v, %v; want %v", candidate, got, err, want)
		}
	}
}

func TestValidateUser(t *testing.T) {
	if testStore == nil {
		t.Skip("DB not available")
	}
	username := uuid.NewString()
	seedUser(t, username, "test-password")
	for _, test := range []struct {
		username, password string
		valid              bool
	}{
		{username, "test-password", true},
		{username, "wrong-password", false},
		{uuid.NewString(), "test-password", false},
		{username, "", false},
	} {
		valid, err := testStore.CheckAuthData(test.username, test.password)
		if err != nil || valid != test.valid {
			t.Errorf("CheckAuthData = %v, %v; want %v", valid, err, test.valid)
		}
	}
}
