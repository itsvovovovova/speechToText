package api

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"speechToText/src/cache"
	"speechToText/src/db"
	"speechToText/src/service"
	"speechToText/src/types"
)

// Handlers holds all service dependencies for HTTP handlers.
type Handlers struct {
	store   *db.Store
	session *cache.RedisSessionManager
}

func NewHandlers(store *db.Store, session *cache.RedisSessionManager) *Handlers {
	return &Handlers{store: store, session: session}
}

func validateAudioURL(audioURL string) error {
	if audioURL == "" {
		return fmt.Errorf("audio URL is required")
	}
	u, err := url.Parse(audioURL)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Hostname() == "" || u.User != nil {
		return fmt.Errorf("invalid audio URL: must be an http or https URL")
	}
	return nil
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	response, err := json.Marshal(v)
	if err != nil {
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if _, err := w.Write(response); err != nil {
		service.LogError("Write error: %v", err)
	}
}

// Audio godoc
// @Summary Upload audio for processing
// @Description Sends audio URL for speech to text conversion
// @Tags audio
// @Accept json
// @Produce json
// @Security ApiKeyAuth
// @Param request body types.AudioRequest true "Audio URL"
// @Param Idempotency-Key header string false "Reuse with the same audio URL to get the original task ID"
// @Failure 409 {string} string "Idempotency key already used"
// @Success 202 {object} types.GetInfoResponse "Task ID created"
// @Failure 400 {string} string "Validation error"
// @Failure 401 {string} string "Unauthorized"
// @Failure 500 {string} string "Internal server error"
// @Router /audio [post]
func (h *Handlers) Audio(w http.ResponseWriter, r *http.Request) {
	defer r.Body.Close()

	session, err := h.session.SessionGet(r.Context(), r)
	if err != nil {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}
	username, err := session.Get(r.Context(), "username")
	if err != nil || username == "" {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}
	data, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 16<<10))
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	var request types.AudioRequest
	if err = json.Unmarshal(data, &request); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if err = validateAudioURL(request.Audio); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	key := r.Header.Get("Idempotency-Key")
	if len(key) > 128 {
		http.Error(w, "Idempotency-Key too long", http.StatusBadRequest)
		return
	}
	taskID, err := h.store.Enqueue(r.Context(), uuid.NewString(), username, request.Audio, key)
	if errors.Is(err, db.ErrIdempotencyConflict) {
		http.Error(w, err.Error(), http.StatusConflict)
		return
	}
	if err != nil {
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Location", "/tasks/"+taskID)
	writeJSON(w, http.StatusAccepted, types.GetInfoResponse{Task_id: taskID})
}

// Status godoc
// @Summary Get task status
// @Description Returns current audio processing status
// @Tags tasks
// @Accept json
// @Produce json
// @Security ApiKeyAuth
// @Param task_id query string true "Task ID"
// @Success 200 {object} types.GetStatusResponse "Task status"
// @Failure 400 {string} string "Validation error"
// @Failure 401 {string} string "Unauthorized"
// @Failure 404 {string} string "Task not found"
// @Failure 500 {string} string "Internal server error"
// @Router /status [get]
func (h *Handlers) Status(w http.ResponseWriter, r *http.Request) {
	session, err := h.session.SessionGet(r.Context(), r)
	if err != nil {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}
	username, err := session.Get(r.Context(), "username")
	if err != nil || username == "" {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}
	taskID := r.URL.Query().Get("task_id")
	if taskID == "" {
		http.Error(w, "task_id is required", http.StatusBadRequest)
		return
	}
	status, _, err := h.store.ReadTask(r.Context(), taskID, username)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			http.Error(w, "task not found", http.StatusNotFound)
			return
		}
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, types.GetStatusResponse{Status: status})
}

// Result godoc
// @Summary Get processing result
// @Description Returns audio to text conversion result
// @Tags tasks
// @Accept json
// @Produce json
// @Security ApiKeyAuth
// @Param task_id query string true "Task ID"
// @Success 200 {object} types.GetResultResponse "Processing result"
// @Failure 400 {string} string "Validation error"
// @Failure 401 {string} string "Unauthorized"
// @Failure 404 {string} string "Task not found"
// @Failure 500 {string} string "Internal server error"
// @Router /result [get]
func (h *Handlers) Result(w http.ResponseWriter, r *http.Request) {
	session, err := h.session.SessionGet(r.Context(), r)
	if err != nil {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}
	username, err := session.Get(r.Context(), "username")
	if err != nil || username == "" {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}
	taskID := chi.URLParam(r, "id")
	if taskID == "" {
		taskID = r.URL.Query().Get("task_id")
	}
	if taskID == "" {
		http.Error(w, "task_id is required", http.StatusBadRequest)
		return
	}
	status, result, err := h.store.ReadTask(r.Context(), taskID, username)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			http.Error(w, "task not found", http.StatusNotFound)
			return
		}
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}
	response := types.GetResultResponse{Status: status, Result: result}
	if status == "failed" {
		response.Error = "transcription_failed"
	}
	writeJSON(w, http.StatusOK, response)
}

// GetTask godoc
// @Summary Get transcription task
// @Tags tasks
// @Produce json
// @Security ApiKeyAuth
// @Param id path string true "Task ID"
// @Success 200 {object} types.GetResultResponse
// @Failure 401 {string} string "Unauthorized"
// @Failure 404 {string} string "Task not found"
// @Failure 500 {string} string "Internal server error"
// @Router /tasks/{id} [get]
func (h *Handlers) GetTask(w http.ResponseWriter, r *http.Request) {
	h.Result(w, r)
}

// Tasks godoc
// @Summary Get tasks list with pagination
// @Description Returns user tasks list with pagination
// @Tags tasks
// @Accept json
// @Produce json
// @Security ApiKeyAuth
// @Param page query int false "Page number" default(1)
// @Param page_size query int false "Page size" default(10)
// @Success 200 {object} types.TaskListResponse "Tasks list"
// @Failure 401 {string} string "Unauthorized"
// @Failure 500 {string} string "Internal server error"
// @Router /tasks [get]
func (h *Handlers) Tasks(w http.ResponseWriter, r *http.Request) {
	session, err := h.session.SessionGet(r.Context(), r)
	if err != nil {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}
	username, err := session.Get(r.Context(), "username")
	if err != nil || username == "" {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	page := 1
	pageSize := 10

	if pageStr := r.URL.Query().Get("page"); pageStr != "" {
		if p, err := strconv.Atoi(pageStr); err == nil && p > 0 {
			page = p
		}
	}
	if pageSizeStr := r.URL.Query().Get("page_size"); pageSizeStr != "" {
		if ps, err := strconv.Atoi(pageSizeStr); err == nil && ps > 0 && ps <= 100 {
			pageSize = ps
		}
	}

	tasks, total, err := h.store.GetTasksWithPagination(username, page, pageSize)
	if err != nil {
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	totalPages := int((total + int64(pageSize) - 1) / int64(pageSize))
	writeJSON(w, http.StatusOK, types.TaskListResponse{
		Tasks: tasks,
		Pagination: types.PaginationResponse{
			Page:       page,
			PageSize:   pageSize,
			Total:      total,
			TotalPages: totalPages,
		},
	})
}

// DeleteTask godoc
// @Summary Delete a task
// @Description Deletes a task by ID; only the owner can delete
// @Tags tasks
// @Accept json
// @Produce json
// @Security ApiKeyAuth
// @Param id path string true "Task ID"
// @Success 200 {object} map[string]string "Task deleted"
// @Failure 401 {string} string "Unauthorized"
// @Failure 404 {string} string "Task not found"
// @Failure 500 {string} string "Internal server error"
// @Router /tasks/{id} [delete]
func (h *Handlers) DeleteTask(w http.ResponseWriter, r *http.Request) {
	session, err := h.session.SessionGet(r.Context(), r)
	if err != nil {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}
	username, err := session.Get(r.Context(), "username")
	if err != nil || username == "" {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}
	taskID := chi.URLParam(r, "id")
	if taskID == "" {
		http.Error(w, "task id is required", http.StatusBadRequest)
		return
	}
	if err := h.store.DeleteTask(r.Context(), taskID, username); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			http.Error(w, "task not found", http.StatusNotFound)
			return
		}
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"result": "ok"})
}
