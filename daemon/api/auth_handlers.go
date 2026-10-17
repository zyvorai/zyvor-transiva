// Copyright 2026 Zyvor AI Labs · https://zyvor.dev
// SPDX-License-Identifier: Apache-2.0

package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/zyvorai/transiva/daemon/auth"
)

// LoginRequest represents a login request
type LoginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

// LoginResponse represents a login response
type LoginResponse struct {
	Token     string `json:"token"`
	Username  string `json:"username"`
	ExpiresAt string `json:"expires_at"`
}

// handleLogin handles user login
func (es *EnhancedServer) handleLogin(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req LoginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	// Authenticate user
	session, err := es.authMgr.AuthenticateUser(req.Username, req.Password)
	if err != nil {
		switch {
		case errors.Is(err, auth.ErrVerifierUnavailable):
			http.Error(w, "sign-in is unavailable on this server: password verification is not configured", http.StatusServiceUnavailable)
		case errors.Is(err, auth.ErrTooManyAttempts):
			http.Error(w, err.Error(), http.StatusTooManyRequests)
		default:
			http.Error(w, "invalid username or password", http.StatusUnauthorized)
		}
		return
	}

	response := LoginResponse{
		Token:     session.Token,
		Username:  session.Username,
		ExpiresAt: session.ExpiresAt.Format("2006-01-02T15:04:05Z07:00"),
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(response); err != nil {
		es.logger.Error("failed to encode login response", "error", err)
	}
}

// handleLogout handles user logout
func (es *EnhancedServer) handleLogout(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	token := r.Header.Get("Authorization")
	if token == "" {
		http.Error(w, "missing authorization token", http.StatusUnauthorized)
		return
	}

	// Remove "Bearer " prefix if present
	if len(token) > 7 && token[:7] == "Bearer " {
		token = token[7:]
	}

	es.authMgr.Logout(token)

	w.WriteHeader(http.StatusOK)
	if err := json.NewEncoder(w).Encode(map[string]string{"message": "logged out successfully"}); err != nil {
		es.logger.Error("failed to encode logout response", "error", err)
	}
}

// handleSession reports whether the caller's session token is still valid. The dashboard uses it
// to restore a signed-in session after a page refresh without asking for the password again.
func (es *EnhancedServer) handleSession(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	token := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
	session, err := es.authMgr.ValidateSession(token)
	if err != nil {
		http.Error(w, "session is not valid", http.StatusUnauthorized)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(map[string]string{
		"username":   session.Username,
		"expires_at": session.ExpiresAt.Format("2006-01-02T15:04:05Z07:00"),
	}); err != nil {
		es.logger.Error("failed to encode session response", "error", err)
	}
}
