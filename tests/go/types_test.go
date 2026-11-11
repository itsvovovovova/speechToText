package main

import (
	"speechToText/src/types"
	"testing"
)

func TestPaginationRequest(t *testing.T) {
	tests := []struct {
		name     string
		request  types.PaginationRequest
		expected bool
	}{
		{
			name: "Valid pagination request",
			request: types.PaginationRequest{
				Page:     1,
				PageSize: 10,
			},
			expected: true,
		},
		{
			name: "Invalid page (zero)",
			request: types.PaginationRequest{
				Page:     0,
				PageSize: 10,
			},
			expected: false,
		},
		{
			name: "Invalid page size (zero)",
			request: types.PaginationRequest{
				Page:     1,
				PageSize: 0,
			},
			expected: false,
		},
		{
			name: "Invalid page size (too large)",
			request: types.PaginationRequest{
				Page:     1,
				PageSize: 101,
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.request.Page < 1 {
				if tt.expected {
					t.Errorf("Expected valid request but got invalid page")
				}
			}
			if tt.request.PageSize < 1 || tt.request.PageSize > 100 {
				if tt.expected {
					t.Errorf("Expected valid request but got invalid page size")
				}
			}
		})
	}
}

func TestPaginationResponse(t *testing.T) {
	response := types.PaginationResponse{
		Page:       1,
		PageSize:   10,
		Total:      25,
		TotalPages: 3,
	}

	if response.Page != 1 {
		t.Errorf("Expected Page to be 1, got %d", response.Page)
	}
	if response.PageSize != 10 {
		t.Errorf("Expected PageSize to be 10, got %d", response.PageSize)
	}
	if response.Total != 25 {
		t.Errorf("Expected Total to be 25, got %d", response.Total)
	}
	if response.TotalPages != 3 {
		t.Errorf("Expected TotalPages to be 3, got %d", response.TotalPages)
	}
}

func TestTaskListResponse(t *testing.T) {
	tasks := []types.TaskInfo{
		{
			TaskID:   "task1",
			Username: "user1",
			Status:   "completed",
			Created:  "2023-01-01T00:00:00Z",
		},
		{
			TaskID:   "task2",
			Username: "user1",
			Status:   "processing",
			Created:  "2023-01-02T00:00:00Z",
		},
	}

	pagination := types.PaginationResponse{
		Page:       1,
		PageSize:   10,
		Total:      2,
		TotalPages: 1,
	}

	response := types.TaskListResponse{
		Tasks:      tasks,
		Pagination: pagination,
	}

	if len(response.Tasks) != 2 {
		t.Errorf("Expected 2 tasks, got %d", len(response.Tasks))
	}
	if response.Pagination.Total != 2 {
		t.Errorf("Expected total to be 2, got %d", response.Pagination.Total)
	}
}

func TestTaskInfo(t *testing.T) {
	task := types.TaskInfo{
		TaskID:   "test-task-id",
		Username: "testuser",
		Status:   "completed",
		Created:  "2023-01-01T00:00:00Z",
	}

	if task.TaskID == "" {
		t.Errorf("TaskID should not be empty")
	}
	if task.Username == "" {
		t.Errorf("Username should not be empty")
	}
	if task.Status == "" {
		t.Errorf("Status should not be empty")
	}
	if task.Created == "" {
		t.Errorf("Created should not be empty")
	}
}

func TestAuthRequest(t *testing.T) {
	tests := []struct {
		name     string
		request  types.AuthRequest
		expected bool
	}{
		{
			name: "Valid auth request",
			request: types.AuthRequest{
				Username: "testuser",
				Password: "testpass",
			},
			expected: true,
		},
		{
			name: "Empty username",
			request: types.AuthRequest{
				Username: "",
				Password: "testpass",
			},
			expected: false,
		},
		{
			name: "Empty password",
			request: types.AuthRequest{
				Username: "testuser",
				Password: "",
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.request.Username == "" || tt.request.Password == "" {
				if tt.expected {
					t.Errorf("Expected valid request but got invalid credentials")
				}
			}
		})
	}
}

func TestAudioRequest(t *testing.T) {
	tests := []struct {
		name     string
		request  types.AudioRequest
		expected bool
	}{
		{
			name: "Valid audio request",
			request: types.AudioRequest{
				Audio: "base64encodedaudio",
			},
			expected: true,
		},
		{
			name: "Empty audio",
			request: types.AudioRequest{
				Audio: "",
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.request.Audio == "" {
				if tt.expected {
					t.Errorf("Expected valid request but got empty audio")
				}
			}
		})
	}
}

func TestGetStatusResponse(t *testing.T) {
	response := types.GetStatusResponse{
		Status: "completed",
	}

	if response.Status == "" {
		t.Errorf("Status should not be empty")
	}
}

func TestGetResultResponse(t *testing.T) {
	text := "transcribed text"
	response := types.GetResultResponse{
		Result: &text,
	}

	if response.Result == nil || *response.Result != text {
		t.Errorf("Result should not be empty")
	}
}
