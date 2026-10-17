// Copyright 2026 Zyvor AI Labs · https://zyvor.dev
// SPDX-License-Identifier: Apache-2.0

package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"sync"
	"time"
)

const (
	// sessionIdleTTL is how long a session stays valid without use; each use renews it.
	sessionIdleTTL = 24 * time.Hour
	// sessionMaxAge caps a session's total lifetime however often it is used.
	sessionMaxAge = 7 * 24 * time.Hour
	// renewThreshold avoids rewriting the session file on every request.
	renewThreshold = 5 * time.Minute
)

// usernamePattern restricts usernames passed to external commands (su/id) to
// the POSIX portable filename character set, and requires the first
// character to not be a hyphen so the value can never be mistaken for a
// command-line flag by the invoked binary.
var usernamePattern = regexp.MustCompile(`^[a-zA-Z0-9_][a-zA-Z0-9._-]{0,31}$`)

// isValidUsername reports whether username is safe to pass as a positional
// argument to external commands such as `id`.
func isValidUsername(username string) bool {
	return usernamePattern.MatchString(username)
}

// Session represents a user session
type Session struct {
	Token     string
	Username  string
	CreatedAt time.Time
	ExpiresAt time.Time
	// IdleTTL, when non-zero, makes the session sliding: each validation pushes
	// ExpiresAt out to now+IdleTTL, up to CreatedAt+sessionMaxAge.
	IdleTTL time.Duration
}

// AuthManager handles user authentication
type AuthManager struct {
	// sessions is keyed by sha256(token), so a persisted file never holds a usable token.
	sessions map[string]*Session
	mu       sync.RWMutex
	path     string // optional: where sessions are persisted across restarts

	verifier PasswordVerifier
	failMu   sync.Mutex
	failures map[string]*loginFailures
}

// loginFailures throttles repeated bad passwords for one account.
type loginFailures struct {
	count int
	first time.Time
	until time.Time // locked until this time once the limit is reached
}

const (
	maxLoginFailures   = 5
	loginFailureWindow = 5 * time.Minute
	loginLockout       = 5 * time.Minute
)

// ErrTooManyAttempts is returned while an account is temporarily locked after repeated failures.
var ErrTooManyAttempts = errors.New("too many failed sign-in attempts; try again in a few minutes")

// SetVerifier replaces the password verifier (used by tests and by embedders).
func (am *AuthManager) SetVerifier(v PasswordVerifier) { am.verifier = v }

func (am *AuthManager) loginLocked(username string) bool {
	am.failMu.Lock()
	defer am.failMu.Unlock()
	f := am.failures[username]
	return f != nil && time.Now().Before(f.until)
}

func (am *AuthManager) recordLogin(username string, ok bool) {
	am.failMu.Lock()
	defer am.failMu.Unlock()
	if ok {
		delete(am.failures, username)
		return
	}
	now := time.Now()
	f := am.failures[username]
	if f == nil || now.Sub(f.first) > loginFailureWindow {
		f = &loginFailures{first: now}
		am.failures[username] = f
	}
	f.count++
	if f.count >= maxLoginFailures {
		f.until = now.Add(loginLockout)
		f.count = 0
		f.first = now
	}
}

// NewAuthManager creates an in-memory authentication manager.
func NewAuthManager() *AuthManager {
	am := &AuthManager{
		sessions: make(map[string]*Session),
		verifier: defaultVerifier(),
		failures: make(map[string]*loginFailures),
	}

	// Start cleanup goroutine
	go am.cleanupExpiredSessions()

	return am
}

// NewPersistentAuthManager creates a manager that keeps sessions in path (mode 0600, token
// hashes only) so that signed-in users stay signed in across a daemon restart.
func NewPersistentAuthManager(path string) *AuthManager {
	am := &AuthManager{
		sessions: make(map[string]*Session),
		path:     path,
		verifier: defaultVerifier(),
		failures: make(map[string]*loginFailures),
	}
	am.load()
	go am.cleanupExpiredSessions()
	return am
}

// DefaultSessionFile returns the session file to use: $TRANSIVA_SESSION_FILE, else
// /var/lib/transiva/sessions.json when that directory exists, else "" (in-memory only).
func DefaultSessionFile() string {
	if p := os.Getenv("TRANSIVA_SESSION_FILE"); p != "" {
		return p
	}
	if st, err := os.Stat("/var/lib/transiva"); err == nil && st.IsDir() {
		return "/var/lib/transiva/sessions.json"
	}
	return ""
}

func tokenKey(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

type storedSession struct {
	Key       string        `json:"key"`
	Username  string        `json:"username"`
	CreatedAt time.Time     `json:"created_at"`
	ExpiresAt time.Time     `json:"expires_at"`
	IdleTTL   time.Duration `json:"idle_ttl"`
}

// load reads persisted sessions, dropping any that have expired. Best effort: a missing or
// unreadable file just means everyone signs in again.
func (am *AuthManager) load() {
	if am.path == "" {
		return
	}
	data, err := os.ReadFile(am.path)
	if err != nil {
		return
	}
	var stored []storedSession
	if err := json.Unmarshal(data, &stored); err != nil {
		log.Printf("auth: ignoring unreadable session file %s: %v", am.path, err)
		return
	}
	now := time.Now()
	for _, st := range stored {
		if now.After(st.ExpiresAt) {
			continue
		}
		am.sessions[st.Key] = &Session{Username: st.Username, CreatedAt: st.CreatedAt, ExpiresAt: st.ExpiresAt, IdleTTL: st.IdleTTL}
	}
}

// persistLocked writes the session table atomically. The caller must hold am.mu.
func (am *AuthManager) persistLocked() {
	if am.path == "" {
		return
	}
	stored := make([]storedSession, 0, len(am.sessions))
	for key, s := range am.sessions {
		stored = append(stored, storedSession{Key: key, Username: s.Username, CreatedAt: s.CreatedAt, ExpiresAt: s.ExpiresAt, IdleTTL: s.IdleTTL})
	}
	data, err := json.Marshal(stored)
	if err != nil {
		return
	}
	tmp := am.path + ".tmp"
	if err := os.MkdirAll(filepath.Dir(am.path), 0o750); err != nil {
		log.Printf("auth: cannot persist sessions: %v", err)
		return
	}
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		log.Printf("auth: cannot persist sessions: %v", err)
		return
	}
	if err := os.Rename(tmp, am.path); err != nil {
		log.Printf("auth: cannot persist sessions: %v", err)
	}
}

// AuthenticateUser authenticates a user against /etc/passwd using PAM
func (am *AuthManager) AuthenticateUser(username, password string) (*Session, error) {
	// Reject usernames that aren't safe to pass as positional arguments to
	// external commands (e.g. values that could be interpreted as flags, or
	// that contain characters outside the portable POSIX username set).
	if !isValidUsername(username) {
		return nil, ErrInvalidCredentials
	}

	if am.loginLocked(username) {
		return nil, ErrTooManyAttempts
	}

	// Check the password against the system's accounts (PAM).
	if err := am.verifier.Verify(username, password); err != nil {
		if errors.Is(err, ErrVerifierUnavailable) {
			return nil, err
		}
		am.recordLogin(username, false)
		return nil, ErrInvalidCredentials
	}
	am.recordLogin(username, true)

	// Generate session token
	token, err := generateToken()
	if err != nil {
		return nil, err
	}

	session := &Session{
		Token:     token,
		Username:  username,
		CreatedAt: time.Now(),
		ExpiresAt: time.Now().Add(sessionIdleTTL),
		IdleTTL:   sessionIdleTTL,
	}

	am.mu.Lock()
	am.sessions[tokenKey(token)] = session
	am.persistLocked()
	am.mu.Unlock()

	return session, nil
}

// ValidateSession validates a session token. A valid sliding session is renewed, so an active
// user is not signed out after a fixed time.
func (am *AuthManager) ValidateSession(token string) (*Session, error) {
	am.mu.Lock()
	defer am.mu.Unlock()

	session, ok := am.sessions[tokenKey(token)]
	if !ok {
		return nil, errors.New("invalid session token")
	}

	now := time.Now()
	if now.After(session.ExpiresAt) {
		return nil, errors.New("session expired")
	}

	if session.IdleTTL > 0 {
		next := now.Add(session.IdleTTL)
		if !session.CreatedAt.IsZero() {
			if limit := session.CreatedAt.Add(sessionMaxAge); next.After(limit) {
				next = limit
			}
		}
		if next.Sub(session.ExpiresAt) > renewThreshold {
			session.ExpiresAt = next
			am.persistLocked()
		}
	}

	return session, nil
}

// Logout removes a session
func (am *AuthManager) Logout(token string) {
	am.mu.Lock()
	defer am.mu.Unlock()
	delete(am.sessions, tokenKey(token))
	am.persistLocked()
}

// cleanupExpiredSessions periodically removes expired sessions
func (am *AuthManager) cleanupExpiredSessions() {
	ticker := time.NewTicker(1 * time.Hour)
	defer ticker.Stop()

	for range ticker.C {
		am.mu.Lock()
		now := time.Now()
		removed := false
		for key, session := range am.sessions {
			if now.After(session.ExpiresAt) {
				delete(am.sessions, key)
				removed = true
			}
		}
		if removed {
			am.persistLocked()
		}
		am.mu.Unlock()
	}
}

// generateToken generates a random session token
func generateToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("failed to generate token: %w", err)
	}
	return base64.URLEncoding.EncodeToString(b), nil
}
