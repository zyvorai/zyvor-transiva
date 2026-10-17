// Copyright 2026 Zyvor AI Labs · https://zyvor.dev
// SPDX-License-Identifier: Apache-2.0

package auth

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func newTestSession(token string, idle time.Duration) *Session {
	now := time.Now()
	return &Session{Token: token, Username: "alice", CreatedAt: now, ExpiresAt: now.Add(idle), IdleTTL: idle}
}

func TestSessionsSurviveARestart(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sessions.json")
	first := NewPersistentAuthManager(path)
	s := newTestSession("tok-restart", time.Hour)
	first.mu.Lock()
	first.sessions[tokenKey(s.Token)] = s
	first.persistLocked()
	first.mu.Unlock()

	second := NewPersistentAuthManager(path) // a new process reading the same file
	got, err := second.ValidateSession("tok-restart")
	if err != nil {
		t.Fatalf("session should survive a restart: %v", err)
	}
	if got.Username != "alice" {
		t.Errorf("username = %q, want alice", got.Username)
	}
}

func TestPersistedFileHoldsNoRawTokenAndIsPrivate(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sessions.json")
	am := NewPersistentAuthManager(path)
	s := newTestSession("super-secret-token", time.Hour)
	am.mu.Lock()
	am.sessions[tokenKey(s.Token)] = s
	am.persistLocked()
	am.mu.Unlock()

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "super-secret-token") {
		t.Error("session file must not contain the raw token")
	}
	st, _ := os.Stat(path)
	if st.Mode().Perm() != 0o600 {
		t.Errorf("session file mode = %v, want 0600", st.Mode().Perm())
	}
}

func TestExpiredSessionsAreDroppedOnLoad(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sessions.json")
	am := NewPersistentAuthManager(path)
	old := newTestSession("old", time.Hour)
	old.ExpiresAt = time.Now().Add(-time.Minute)
	am.mu.Lock()
	am.sessions[tokenKey(old.Token)] = old
	am.persistLocked()
	am.mu.Unlock()

	if _, err := NewPersistentAuthManager(path).ValidateSession("old"); err == nil {
		t.Error("an expired session must not be restored")
	}
}

func TestLogoutIsPersisted(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sessions.json")
	am := NewPersistentAuthManager(path)
	s := newTestSession("bye", time.Hour)
	am.mu.Lock()
	am.sessions[tokenKey(s.Token)] = s
	am.persistLocked()
	am.mu.Unlock()
	am.Logout("bye")

	if _, err := NewPersistentAuthManager(path).ValidateSession("bye"); err == nil {
		t.Error("a logged-out session must not come back after a restart")
	}
}

func TestSlidingSessionIsRenewedByUse(t *testing.T) {
	am := NewAuthManager()
	s := newTestSession("slide", 24*time.Hour)
	s.ExpiresAt = time.Now().Add(time.Hour) // an hour left of a 24h idle window
	am.sessions[tokenKey(s.Token)] = s

	got, err := am.ValidateSession("slide")
	if err != nil {
		t.Fatal(err)
	}
	if remaining := time.Until(got.ExpiresAt); remaining < 23*time.Hour {
		t.Errorf("active session should be renewed to about 24h, has %v left", remaining)
	}
}

func TestSlidingSessionIsCappedAtMaxAge(t *testing.T) {
	am := NewAuthManager()
	s := newTestSession("capped", 24*time.Hour)
	s.CreatedAt = time.Now().Add(-sessionMaxAge + time.Hour) // one hour from the absolute limit
	s.ExpiresAt = time.Now().Add(30 * time.Minute)
	am.sessions[tokenKey(s.Token)] = s

	got, err := am.ValidateSession("capped")
	if err != nil {
		t.Fatal(err)
	}
	if limit := s.CreatedAt.Add(sessionMaxAge); got.ExpiresAt.After(limit) {
		t.Errorf("session extended past its absolute limit: %v > %v", got.ExpiresAt, limit)
	}
}

func TestNonSlidingSessionKeepsItsExpiry(t *testing.T) {
	am := NewAuthManager()
	s := &Session{Token: "fixed", Username: "bob", ExpiresAt: time.Now().Add(2 * time.Second)}
	am.sessions[tokenKey(s.Token)] = s
	before := s.ExpiresAt
	if _, err := am.ValidateSession("fixed"); err != nil {
		t.Fatal(err)
	}
	if !s.ExpiresAt.Equal(before) {
		t.Error("a session without IdleTTL must not be extended")
	}
}
