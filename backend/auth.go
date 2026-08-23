package main

import (
	"crypto/rand"
	"encoding/hex"
	"log"
	"net/http"
	"sync"
)

type app struct {
	users    *userStore
	allowed  map[int64]struct{}
	sessions map[string]int64
	mu       sync.RWMutex
}

func (a *app) isAuthenticated(r *http.Request) bool {
	cookie, err := r.Cookie("session")
	if err != nil {
		return false
	}

	a.mu.RLock()
	userID, ok := a.sessions[cookie.Value]
	a.mu.RUnlock()
	if !ok {
		return false
	}

	_, allowed := a.allowed[userID]
	return allowed
}

func (a *app) rootHandler(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}

	w.Header().Set("Cache-Control", "no-store")
	page := "frontend/html/login.html"
	if a.isAuthenticated(r) {
		page = "frontend/html/index.html"
	}
	http.ServeFile(w, r, page)
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
		http.Error(w, "could not create session", http.StatusInternalServerError)
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

func newSessionToken() (string, error) {
	bytes := make([]byte, 32)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return hex.EncodeToString(bytes), nil
}
