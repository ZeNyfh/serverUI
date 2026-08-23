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

func decodeCredentials(w http.ResponseWriter, r *http.Request, input *credentials) bool {
	if err := json.NewDecoder(r.Body).Decode(input); err != nil {
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return false
	}
	return true
}
