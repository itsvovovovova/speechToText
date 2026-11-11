package service

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"speechToText/src/types"
)

func LogDebug(format string, args ...interface{}) {
	log.Printf("[DEBUG] "+format, args...)
}

func LogInfo(format string, args ...interface{}) {
	log.Printf("[INFO] "+format, args...)
}

func LogError(format string, args ...interface{}) {
	log.Printf("[ERROR] "+format, args...)
}

func ReadAuthRequest(r *http.Request) (types.AuthRequest, error) {
	data, err := io.ReadAll(io.LimitReader(r.Body, 16<<10+1))
	if err != nil {
		return types.AuthRequest{}, err
	}
	if len(data) > 16<<10 {
		return types.AuthRequest{}, fmt.Errorf("request too large")
	}
	var authData types.AuthRequest
	if err = json.Unmarshal(data, &authData); err != nil {
		return types.AuthRequest{}, err
	}
	if authData.Username == "" || authData.Password == "" {
		return types.AuthRequest{}, fmt.Errorf("username and password are required")
	}
	if len(authData.Password) < 8 {
		return types.AuthRequest{}, fmt.Errorf("password must be at least 8 characters")
	}
	return authData, nil
}
