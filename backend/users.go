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

	return &userStore{db: db}, nil
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
