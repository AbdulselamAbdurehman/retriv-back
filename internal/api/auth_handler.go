package api

import (
	"errors"
	"net/http"
	"strings"

	"github.com/AbdulselamAbdurehman/retriv-back/internal/auth"
)

type registerRequest struct {
	Email      string `json:"email"`
	FirstName  string `json:"first_name"`
	MiddleName string `json:"middle_name"`
	LastName   string `json:"last_name"`
	Phone      string `json:"phone"`
}

func (a *API) handleRegister(w http.ResponseWriter, r *http.Request) {
	var req registerRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Email == "" || req.FirstName == "" || req.LastName == "" || req.Phone == "" {
		writeError(w, http.StatusBadRequest, "email, first_name, last_name and phone are required")
		return
	}
	err := a.auth.Register(r.Context(), auth.RegisterInput{
		Email:      req.Email,
		FirstName:  req.FirstName,
		MiddleName: req.MiddleName,
		LastName:   req.LastName,
		Phone:      req.Phone,
	})
	if errors.Is(err, auth.ErrEmailTaken) {
		writeError(w, http.StatusConflict, "email already registered")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not register")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"message": "check your email for a sign-in link"})
}

type loginRequest struct {
	Email string `json:"email"`
}

func (a *API) handleLogin(w http.ResponseWriter, r *http.Request) {
	var req loginRequest
	if err := decodeJSON(r, &req); err != nil || strings.TrimSpace(req.Email) == "" {
		writeError(w, http.StatusBadRequest, "email is required")
		return
	}
	// Always report success to avoid revealing whether the email is registered.
	if err := a.auth.RequestLogin(r.Context(), req.Email); err != nil {
		writeError(w, http.StatusInternalServerError, "could not process login")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"message": "if that email is registered, a sign-in link was sent"})
}

func (a *API) handleVerify(w http.ResponseWriter, r *http.Request) {
	token := r.URL.Query().Get("token")
	if token == "" {
		writeError(w, http.StatusBadRequest, "missing token")
		return
	}
	session, err := a.auth.Verify(r.Context(), token)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "invalid or expired link")
		return
	}
	a.setSessionCookie(w, session)
	http.Redirect(w, r, a.appBaseURL, http.StatusFound)
}

func (a *API) handleLogout(w http.ResponseWriter, r *http.Request) {
	a.clearSessionCookie(w)
	writeJSON(w, http.StatusOK, map[string]string{"message": "signed out"})
}

func (a *API) handleMe(w http.ResponseWriter, r *http.Request) {
	u, err := a.auth.User(r.Context(), userID(r))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not load user")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"id":          u.ID,
		"email":       u.Email,
		"first_name":  u.FirstName,
		"middle_name": u.MiddleName,
		"last_name":   u.LastName,
		"phone":       u.Phone,
	})
}
