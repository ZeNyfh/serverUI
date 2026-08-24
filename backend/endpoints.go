package main

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"
)

type credentials struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type accountUpdate struct {
	Username        string `json:"username"`
	CurrentPassword string `json:"currentPassword"`
	NewPassword     string `json:"newPassword"`
}

func (s *userStore) createUserHandler(w http.ResponseWriter, r *http.Request) {
	var input credentials
	if !decodeCredentials(w, r, &input) {
		return
	}

	_, err := s.createUser(r.Context(), input.Username, input.Password)
	if errors.Is(err, ErrUsernameTaken) {
		http.Error(w, err.Error(), http.StatusConflict)
		return
	}
	if err != nil {
		http.Error(w, "could not create user", http.StatusBadRequest)
		return
	}

	w.WriteHeader(http.StatusCreated)
}

func (a *app) loginHandler(w http.ResponseWriter, r *http.Request) {
	var input credentials
	if !decodeCredentials(w, r, &input) {
		log.Println("login failed")
		return
	}

	userID, err := a.users.authenticate(r.Context(), input.Username, input.Password)
	if err != nil {
		log.Println("login failed")
		http.Error(w, "login failed", http.StatusUnauthorized)
		return
	}
	if _, allowed := a.allowed[userID]; !allowed {
		log.Println("login failed")
		http.Error(w, "login failed", http.StatusForbidden)
		return
	}

	token, err := newSessionToken()
	if err != nil {
		log.Println("login failed")
		http.Error(w, "login failed", http.StatusInternalServerError)
		return
	}

	a.mu.Lock()
	a.sessions[token] = userID
	a.mu.Unlock()

	http.SetCookie(w, &http.Cookie{
		Name:     "session",
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})
	w.WriteHeader(http.StatusNoContent)
}

func (a *app) currentUserHandler(w http.ResponseWriter, r *http.Request) {
	userID, ok := a.currentUserID(r)
	if !ok {
		http.Error(w, "login failed", http.StatusUnauthorized)
		return
	}

	username, err := a.users.username(r.Context(), userID)
	if err != nil {
		http.Error(w, "could not load account", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"username": username})
}

func (a *app) updateAccountHandler(w http.ResponseWriter, r *http.Request) {
	userID, ok := a.currentUserID(r)
	if !ok {
		http.Error(w, "login failed", http.StatusUnauthorized)
		return
	}

	var input accountUpdate
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return
	}
	if input.CurrentPassword == "" {
		http.Error(w, "current password is required", http.StatusBadRequest)
		return
	}

	err := a.users.updateAccount(r.Context(), userID, input.Username, input.CurrentPassword, input.NewPassword)
	if errors.Is(err, ErrCurrentPassword) {
		http.Error(w, "current password is incorrect", http.StatusUnauthorized)
		return
	}
	if errors.Is(err, ErrUsernameTaken) {
		http.Error(w, err.Error(), http.StatusConflict)
		return
	}
	if err != nil {
		http.Error(w, "could not update account", http.StatusBadRequest)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func decodeCredentials(w http.ResponseWriter, r *http.Request, input *credentials) bool {
	if err := json.NewDecoder(r.Body).Decode(input); err != nil {
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return false
	}
	return true
}
