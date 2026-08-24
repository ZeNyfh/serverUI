package main

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"net/http"
	"time"
)

type app struct {
	users              *userStore
	allowed            map[int64]struct{}
	consolePermissions map[string]map[int64]struct{}
	ssh                *sshConnectionConfig
	sessionTTL         time.Duration
}

func (a *app) isAuthenticated(r *http.Request) bool {
	_, ok := a.currentUserID(r)
	return ok
}

func (a *app) currentUserID(r *http.Request) (int64, bool) {
	cookie, err := r.Cookie("session")
	if err != nil || cookie.Value == "" {
		return 0, false
	}
	userID, ok := a.users.sessionUserID(r.Context(), hashSessionToken(cookie.Value), time.Now().Unix())
	if !ok {
		return 0, false
	}
	_, allowed := a.allowed[userID]
	return userID, allowed
}

func (a *app) setSessionCookie(w http.ResponseWriter, r *http.Request, token string) {
	// Remove the legacy Console password cookie from older deployments.
	http.SetCookie(w, &http.Cookie{Name: "console_ssh_password", Value: "", Path: "/api/console/", HttpOnly: true, Secure: true, SameSite: http.SameSiteStrictMode, MaxAge: -1, Expires: time.Unix(1, 0)})
	http.SetCookie(w, &http.Cookie{
		Name:     "session",
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		Secure:   true,
		SameSite: http.SameSiteStrictMode,
		MaxAge:   int(a.sessionTTL.Seconds()),
		Expires:  time.Now().Add(a.sessionTTL),
	})
}

func clearSessionCookie(w http.ResponseWriter, r *http.Request) {
	http.SetCookie(w, &http.Cookie{Name: "session", Value: "", Path: "/", HttpOnly: true, Secure: true, SameSite: http.SameSiteStrictMode, MaxAge: -1, Expires: time.Unix(1, 0)})
}

func newSessionToken() (string, error) {
	bytes := make([]byte, 32)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(bytes), nil
}

func hashSessionToken(token string) string {
	hash := sha256.Sum256([]byte(token))
	return base64.RawURLEncoding.EncodeToString(hash[:])
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

func sameOrigin(r *http.Request) bool {
	origin := r.Header.Get("Origin")
	return origin == "" || origin == "http://"+r.Host || origin == "https://"+r.Host
}
