package main

import (
	"context"
	"database/sql"
	"errors"
	"strings"

	"golang.org/x/crypto/bcrypt"
	_ "modernc.org/sqlite"
)

var ErrUsernameTaken = errors.New("username is already in use")
var ErrCurrentPassword = errors.New("current password is incorrect")

type userStore struct {
	db *sql.DB
}

func openUserStore(path string) (*userStore, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}

	_, err = db.Exec(`
		CREATE TABLE IF NOT EXISTS users (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			username TEXT NOT NULL UNIQUE,
			password_hash TEXT NOT NULL
		)
	`)
	if err != nil {
		db.Close()
		return nil, err
	}
	_, err = db.Exec(`
		CREATE TABLE IF NOT EXISTS sessions (
			token_hash TEXT PRIMARY KEY,
			user_id INTEGER NOT NULL REFERENCES users(id),
			created_at INTEGER NOT NULL,
			expires_at INTEGER NOT NULL,
			last_seen INTEGER NOT NULL,
			revoked_at INTEGER
		)
	`)
	if err != nil {
		db.Close()
		return nil, err
	}

	return &userStore{db: db}, nil
}

func (s *userStore) createSession(ctx context.Context, tokenHash string, userID int64, createdAt, expiresAt int64) error {
	_, err := s.db.ExecContext(ctx, `INSERT INTO sessions (token_hash, user_id, created_at, expires_at, last_seen) VALUES (?, ?, ?, ?, ?)`, tokenHash, userID, createdAt, expiresAt, createdAt)
	return err
}

func (s *userStore) sessionUserID(ctx context.Context, tokenHash string, now int64) (int64, bool) {
	var userID int64
	err := s.db.QueryRowContext(ctx, `SELECT user_id FROM sessions WHERE token_hash = ? AND revoked_at IS NULL AND expires_at > ?`, tokenHash, now).Scan(&userID)
	if err != nil {
		return 0, false
	}
	_, _ = s.db.ExecContext(ctx, `UPDATE sessions SET last_seen = ? WHERE token_hash = ?`, now, tokenHash)
	return userID, true
}

func (s *userStore) revokeSession(ctx context.Context, tokenHash string, now int64) error {
	_, err := s.db.ExecContext(ctx, `UPDATE sessions SET revoked_at = ? WHERE token_hash = ? AND revoked_at IS NULL`, now, tokenHash)
	return err
}

func (s *userStore) close() error {
	return s.db.Close()
}

func (s *userStore) createUser(ctx context.Context, username, password string) (int64, error) {
	username = strings.TrimSpace(username)
	if username == "" || password == "" {
		return 0, errors.New("username and password are required")
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return 0, err
	}

	result, err := s.db.ExecContext(ctx,
		"INSERT INTO users (username, password_hash) VALUES (?, ?)", username, string(hash))
	if err != nil && strings.Contains(err.Error(), "UNIQUE constraint failed") {
		return 0, ErrUsernameTaken
	}
	if err != nil {
		return 0, err
	}
	return result.LastInsertId()
}

func (s *userStore) authenticate(ctx context.Context, username, password string) (int64, error) {
	var (
		id   int64
		hash string
	)
	err := s.db.QueryRowContext(ctx,
		"SELECT id, password_hash FROM users WHERE username = ?", strings.TrimSpace(username)).Scan(&id, &hash)
	if err != nil {
		return 0, err
	}
	if err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)); err != nil {
		return 0, err
	}
	return id, nil
}

func (s *userStore) username(ctx context.Context, id int64) (string, error) {
	var username string
	err := s.db.QueryRowContext(ctx, "SELECT username FROM users WHERE id = ?", id).Scan(&username)
	return username, err
}

func (s *userStore) updateAccount(ctx context.Context, id int64, username, currentPassword, newPassword string) error {
	var existingUsername, existingHash string
	err := s.db.QueryRowContext(ctx,
		"SELECT username, password_hash FROM users WHERE id = ?", id).Scan(&existingUsername, &existingHash)
	if err != nil {
		return err
	}
	if err := bcrypt.CompareHashAndPassword([]byte(existingHash), []byte(currentPassword)); err != nil {
		return ErrCurrentPassword
	}

	username = strings.TrimSpace(username)
	if username == "" {
		username = existingUsername
	}
	passwordHash := existingHash
	if newPassword != "" {
		hash, err := bcrypt.GenerateFromPassword([]byte(newPassword), bcrypt.DefaultCost)
		if err != nil {
			return err
		}
		passwordHash = string(hash)
	}
	if username == existingUsername && passwordHash == existingHash {
		return errors.New("no account changes provided")
	}

	_, err = s.db.ExecContext(ctx,
		"UPDATE users SET username = ?, password_hash = ? WHERE id = ?", username, passwordHash, id)
	if err != nil && strings.Contains(err.Error(), "UNIQUE constraint failed") {
		return ErrUsernameTaken
	}
	return err
}
