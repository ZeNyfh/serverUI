package main

import (
	"crypto/rand"
	"encoding/hex"
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
	_, ok := a.currentUserID(r)
	return ok
}

func (a *app) currentUserID(r *http.Request) (int64, bool) {
	cookie, err := r.Cookie("session")
	if err != nil {
		return 0, false
	}

	a.mu.RLock()
	userID, ok := a.sessions[cookie.Value]
	a.mu.RUnlock()
	if !ok {
		return 0, false
	}

	_, allowed := a.allowed[userID]
	return userID, allowed
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

func (a *app) settingsHandler(w http.ResponseWriter, r *http.Request) {
	if !a.isAuthenticated(r) {
		http.Error(w, "login failed", http.StatusUnauthorized)
		return
	}

	w.Header().Set("Cache-Control", "no-store")
	http.ServeFile(w, r, "frontend/html/settings.html")
}

func newSessionToken() (string, error) {
	bytes := make([]byte, 32)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return hex.EncodeToString(bytes), nil
}
