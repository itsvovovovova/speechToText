package api

import (
	"net/http"
	"speechToText/src/db"
	"speechToText/src/service"
)

// Register godoc
// @Summary User registration
// @Description Creates a new user in the system
// @Tags auth
// @Accept json
// @Produce json
// @Param request body types.AuthRequest true "Registration data"
// @Success 200 {object} map[string]string "Successful registration"
// @Failure 400 {string} string "Validation error"
// @Failure 500 {string} string "Internal server error"
// @Router /register [post]
func (h *Handlers) Register(w http.ResponseWriter, r *http.Request) {
	defer r.Body.Close()

	user, err := service.ReadAuthRequest(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	hashPassword, err := db.HashPassword(user.Password)
	if err != nil {
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}
	exist, err := h.store.ExistUsername(user.Username)
	if err != nil {
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}
	if exist {
		http.Error(w, "Username already taken", http.StatusBadRequest)
		return
	}
	if err := h.store.AddAuthData(user.Username, hashPassword); err != nil {
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"result": "ok"})
}

// Login godoc
// @Summary User authentication
// @Description Authenticates user and creates session
// @Tags auth
// @Accept json
// @Produce json
// @Param request body types.AuthRequest true "Authentication data"
// @Success 200 {object} map[string]string "Successful authentication"
// @Failure 400 {string} string "Validation error"
// @Failure 401 {string} string "Invalid credentials"
// @Failure 500 {string} string "Internal server error"
// @Router /login [post]
func (h *Handlers) Login(w http.ResponseWriter, r *http.Request) {
	defer r.Body.Close()

	user, err := service.ReadAuthRequest(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	exist, err := h.store.CheckAuthData(user.Username, user.Password)
	if err != nil {
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}
	if !exist {
		http.Error(w, http.StatusText(http.StatusUnauthorized), http.StatusUnauthorized)
		return
	}
	ctx := r.Context()
	session, err := h.session.SessionStart(ctx, w, r)
	if err != nil {
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}
	if err := session.Set(ctx, "username", user.Username); err != nil {
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{
		"result": "ok",
		"token":  session.SessionId,
	})
}

// Logout godoc
// @Summary Logout
// @Description Invalidates the user session
// @Tags auth
// @Accept json
// @Produce json
// @Security ApiKeyAuth
// @Success 200 {object} map[string]string "Logged out"
// @Failure 401 {string} string "Unauthorized"
// @Failure 500 {string} string "Internal server error"
// @Router /logout [post]
func (h *Handlers) Logout(w http.ResponseWriter, r *http.Request) {
	session, err := h.session.SessionGet(r.Context(), r)
	if err != nil {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}
	if err := h.session.SessionDestroy(r.Context(), w, session.SessionId); err != nil {
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"result": "ok"})
}
