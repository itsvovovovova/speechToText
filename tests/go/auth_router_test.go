package main

import (
	"bytes"
	"encoding/json"
	"net/http/httptest"
	"speechToText/src/types"
	"testing"

	"github.com/google/uuid"
)

func TestRegister(t *testing.T) {
	tests := []struct {
		name           string
		requestBody    types.AuthRequest
		expectedStatus int
		expectedResult string
		needsDB        bool
	}{
		{
			name:           "Valid registration",
			requestBody:    types.AuthRequest{Username: uuid.NewString(), Password: "testpass1"},
			expectedStatus: 200,
			expectedResult: "ok",
			needsDB:        true,
		},
		{
			name:           "Empty username",
			requestBody:    types.AuthRequest{Username: "", Password: "testpass"},
			expectedStatus: 400,
		},
		{
			name:           "Empty password",
			requestBody:    types.AuthRequest{Username: "testuser", Password: ""},
			expectedStatus: 400,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.needsDB && testStore == nil {
				t.Skip("DB not available")
			}
			jsonBody, _ := json.Marshal(tt.requestBody)
			req := httptest.NewRequest("POST", "/register", bytes.NewBuffer(jsonBody))
			req.Header.Set("Content-Type", "application/json")

			rr := httptest.NewRecorder()
			testHandlers.Register(rr, req)

			if status := rr.Code; status != tt.expectedStatus {
				t.Errorf("handler returned wrong status code: got %v want %v", status, tt.expectedStatus)
			}

			if tt.expectedStatus == 200 {
				var response map[string]string
				if err := json.Unmarshal(rr.Body.Bytes(), &response); err != nil {
					t.Errorf("Failed to unmarshal response: %v", err)
				}
				if response["result"] != tt.expectedResult {
					t.Errorf("handler returned wrong result: got %v want %v", response["result"], tt.expectedResult)
				}
			}
		})
	}
}

func TestLogin(t *testing.T) {
	tests := []struct {
		name           string
		requestBody    types.AuthRequest
		expectedStatus int
		expectToken    bool
		needsDB        bool
	}{
		{
			name:           "Valid login",
			requestBody:    types.AuthRequest{Username: uuid.NewString(), Password: "testpass1"},
			expectedStatus: 200,
			expectToken:    true,
			needsDB:        true,
		},
		{
			name:           "Invalid credentials",
			requestBody:    types.AuthRequest{Username: "wronguser", Password: "wrongpass1"},
			expectedStatus: 401,
			needsDB:        true,
		},
		{
			name:           "Empty username",
			requestBody:    types.AuthRequest{Username: "", Password: "testpass"},
			expectedStatus: 400,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.needsDB && testStore == nil {
				t.Skip("DB not available")
			}
			if tt.expectToken {
				seedUser(t, tt.requestBody.Username, tt.requestBody.Password)
			}
			jsonBody, _ := json.Marshal(tt.requestBody)
			req := httptest.NewRequest("POST", "/login", bytes.NewBuffer(jsonBody))
			req.Header.Set("Content-Type", "application/json")

			rr := httptest.NewRecorder()
			testHandlers.Login(rr, req)

			if status := rr.Code; status != tt.expectedStatus {
				t.Errorf("handler returned wrong status code: got %v want %v", status, tt.expectedStatus)
			}

			if tt.expectedStatus == 200 && tt.expectToken {
				var response map[string]string
				if err := json.Unmarshal(rr.Body.Bytes(), &response); err != nil {
					t.Errorf("Failed to unmarshal response: %v", err)
				}
				if _, exists := response["token"]; !exists {
					t.Errorf("Expected token in response but not found")
				}
				if response["result"] != "ok" {
					t.Errorf("Expected result 'ok', got %v", response["result"])
				}
			}
		})
	}
}
