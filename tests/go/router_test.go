package main

import (
	"bytes"
	"net/http/httptest"
	"testing"
)

func TestAudioRequiresSession(t *testing.T) {
	req := httptest.NewRequest("POST", "/audio", bytes.NewBufferString(`{"audio":"https://example.com/clip.wav"}`))
	rec := httptest.NewRecorder()
	testHandlers.Audio(rec, req)
	if rec.Code != 401 {
		t.Fatalf("got %d, want 401", rec.Code)
	}
}

func TestStatus(t *testing.T) {
	tests := []struct {
		name           string
		queryParams    string
		expectedStatus int
	}{
		{name: "Valid status request", queryParams: "?task_id=test-task-id", expectedStatus: 401},
		{name: "Missing task_id", queryParams: "", expectedStatus: 401},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", "/status"+tt.queryParams, nil)
			rr := httptest.NewRecorder()
			testHandlers.Status(rr, req)

			if status := rr.Code; status != tt.expectedStatus {
				t.Errorf("handler returned wrong status code: got %v want %v", status, tt.expectedStatus)
			}
		})
	}
}

func TestResult(t *testing.T) {
	tests := []struct {
		name           string
		queryParams    string
		expectedStatus int
	}{
		{name: "Valid result request", queryParams: "?task_id=test-task-id", expectedStatus: 401},
		{name: "Missing task_id", queryParams: "", expectedStatus: 401},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", "/result"+tt.queryParams, nil)
			rr := httptest.NewRecorder()
			testHandlers.Result(rr, req)

			if status := rr.Code; status != tt.expectedStatus {
				t.Errorf("handler returned wrong status code: got %v want %v", status, tt.expectedStatus)
			}
		})
	}
}

func TestTasks(t *testing.T) {
	tests := []struct {
		name           string
		queryParams    string
		expectedStatus int
		expectTasks    bool
	}{
		{name: "Valid tasks request", queryParams: "?page=1&page_size=10", expectedStatus: 401},
		{name: "Invalid page parameter", queryParams: "?page=0&page_size=10", expectedStatus: 401},
		{name: "Invalid page_size parameter", queryParams: "?page=1&page_size=0", expectedStatus: 401},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", "/tasks"+tt.queryParams, nil)
			rr := httptest.NewRecorder()
			testHandlers.Tasks(rr, req)

			if status := rr.Code; status != tt.expectedStatus {
				t.Errorf("handler returned wrong status code: got %v want %v", status, tt.expectedStatus)
			}
		})
	}
}
