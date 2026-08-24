package main

import (
	"context"
	"crypto/tls"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"
)

func newTestApp(t *testing.T) (*app, int64, int64) {
	t.Helper()
	store, err := openUserStore(filepath.Join(t.TempDir(), "users.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.close() })
	allowedUser, err := store.createUser(context.Background(), "allowed", "password")
	if err != nil {
		t.Fatal(err)
	}
	deniedUser, err := store.createUser(context.Background(), "denied", "password")
	if err != nil {
		t.Fatal(err)
	}
	return &app{
		users:              store,
		allowed:            map[int64]struct{}{allowedUser: {}, deniedUser: {}},
		consolePermissions: map[string]map[int64]struct{}{"local": {allowedUser: {}}},
		ssh:                &sshConnectionConfig{machineID: "local"},
		sessionTTL:         time.Hour,
	}, allowedUser, deniedUser
}

func addTestSession(t *testing.T, app *app, userID int64, expiresAt time.Time) string {
	t.Helper()
	token, err := newSessionToken()
	if err != nil {
		t.Fatal(err)
	}
	if err := app.users.createSession(context.Background(), hashSessionToken(token), userID, time.Now().Unix(), expiresAt.Unix()); err != nil {
		t.Fatal(err)
	}
	return token
}

func requestWithSession(method, target, token string) *http.Request {
	request := httptest.NewRequest(method, target, nil)
	request.AddCookie(&http.Cookie{Name: "session", Value: token})
	return request
}

func TestSessionValidationExpirationAndRevocation(t *testing.T) {
	app, allowedUser, _ := newTestApp(t)
	token := addTestSession(t, app, allowedUser, time.Now().Add(time.Hour))
	var storedHash string
	if err := app.users.db.QueryRow(`SELECT token_hash FROM sessions WHERE user_id = ?`, allowedUser).Scan(&storedHash); err != nil {
		t.Fatal(err)
	}
	if storedHash == token {
		t.Fatal("raw session token was stored")
	}
	if userID, ok := app.currentUserID(requestWithSession(http.MethodGet, "/", token)); !ok || userID != allowedUser {
		t.Fatalf("valid session was not resolved: user_id=%d ok=%t", userID, ok)
	}
	if err := app.users.revokeSession(context.Background(), hashSessionToken(token), time.Now().Unix()); err != nil {
		t.Fatal(err)
	}
	if _, ok := app.currentUserID(requestWithSession(http.MethodGet, "/", token)); ok {
		t.Fatal("revoked session remained valid")
	}
	expired := addTestSession(t, app, allowedUser, time.Now().Add(-time.Minute))
	if _, ok := app.currentUserID(requestWithSession(http.MethodGet, "/", expired)); ok {
		t.Fatal("expired session remained valid")
	}
}

func TestSessionCookieSecurityProperties(t *testing.T) {
	app, _, _ := newTestApp(t)
	request := httptest.NewRequest(http.MethodPost, "https://example.test/api/login", nil)
	request.TLS = &tls.ConnectionState{}
	recorder := httptest.NewRecorder()
	app.setSessionCookie(recorder, request, "opaque-token")
	for _, cookie := range recorder.Result().Cookies() {
		if cookie.Name != "session" {
			continue
		}
		if !cookie.HttpOnly || !cookie.Secure || cookie.SameSite != http.SameSiteStrictMode || cookie.Path != "/" {
			t.Fatalf("unexpected session cookie security properties: %#v", cookie)
		}
		return
	}
	t.Fatal("session cookie was not set")
}

func TestConsoleAuthorization(t *testing.T) {
	app, allowedUser, deniedUser := newTestApp(t)
	allowedToken := addTestSession(t, app, allowedUser, time.Now().Add(time.Hour))
	deniedToken := addTestSession(t, app, deniedUser, time.Now().Add(time.Hour))

	if _, ok := app.authorizeConsole(httptest.NewRecorder(), httptest.NewRequest(http.MethodPost, "/api/console/sessions", nil)); ok {
		t.Fatal("unauthenticated Console request was authorized")
	}
	deniedRecorder := httptest.NewRecorder()
	if _, ok := app.authorizeConsole(deniedRecorder, requestWithSession(http.MethodPost, "/api/console/sessions", deniedToken)); ok || deniedRecorder.Code != http.StatusForbidden {
		t.Fatalf("unauthorized user result: ok=%t status=%d", ok, deniedRecorder.Code)
	}
	allowedRecorder := httptest.NewRecorder()
	if userID, ok := app.authorizeConsole(allowedRecorder, requestWithSession(http.MethodPost, "/api/console/sessions", allowedToken)); !ok || userID != allowedUser {
		t.Fatalf("authorized user was rejected: user_id=%d ok=%t", userID, ok)
	}
	ignoredUserIDRecorder := httptest.NewRecorder()
	if userID, ok := app.authorizeConsole(ignoredUserIDRecorder, requestWithSession(http.MethodPost, "/api/console/sessions?userId="+"999", allowedToken)); !ok || userID != allowedUser {
		t.Fatal("frontend-provided user ID affected Console authorization")
	}
	unknownMachineRecorder := httptest.NewRecorder()
	if _, ok := app.authorizeConsole(unknownMachineRecorder, requestWithSession(http.MethodPost, "/api/console/sessions?machine=other", allowedToken)); ok || unknownMachineRecorder.Code != http.StatusBadRequest {
		t.Fatalf("unknown machine result: ok=%t status=%d", ok, unknownMachineRecorder.Code)
	}
}

func TestConsoleWebSocketAdmissionAndTmuxValidation(t *testing.T) {
	app, allowedUser, deniedUser := newTestApp(t)
	allowedToken := addTestSession(t, app, allowedUser, time.Now().Add(time.Hour))
	deniedToken := addTestSession(t, app, deniedUser, time.Now().Add(time.Hour))

	unauthenticated := httptest.NewRecorder()
	app.consoleTerminalHandler(unauthenticated, httptest.NewRequest(http.MethodGet, "/api/console/terminal", nil))
	if unauthenticated.Code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated WebSocket status=%d", unauthenticated.Code)
	}
	unauthorized := httptest.NewRecorder()
	app.consoleTerminalHandler(unauthorized, requestWithSession(http.MethodGet, "/api/console/terminal", deniedToken))
	if unauthorized.Code != http.StatusForbidden {
		t.Fatalf("unauthorized WebSocket status=%d", unauthorized.Code)
	}
	invalid := httptest.NewRecorder()
	app.consoleTerminalHandler(invalid, requestWithSession(http.MethodGet, "/api/console/terminal?session=bad;id", allowedToken))
	if invalid.Code != http.StatusBadRequest {
		t.Fatalf("invalid tmux session status=%d", invalid.Code)
	}
	if tmuxSessionName.MatchString("bad;id") || !tmuxSessionName.MatchString("valid-session_1") {
		t.Fatal("tmux session validation is unsafe")
	}
}
