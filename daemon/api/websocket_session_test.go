// Copyright 2026 Zyvor AI Labs · https://zyvor.dev
// SPDX-License-Identifier: Apache-2.0

package api

import (
	"encoding/base64"
	"net/http/httptest"
	"testing"
)

func TestWebSocketSessionToken(t *testing.T) {
	token := "session-with-padding="
	encoded := base64.RawURLEncoding.EncodeToString([]byte(token))
	for _, tc := range []struct {
		name, header, want string
	}{
		{"absent", "", ""},
		{"other protocol", "json", ""},
		{"malformed", "transiva-session.!", ""},
		{"session", "transiva-session." + encoded, token},
		{"multiple protocols", "json, transiva-session." + encoded, token},
	} {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", "/ws", nil)
			req.Header.Set("Sec-WebSocket-Protocol", tc.header)
			if got := webSocketSessionToken(req); got != tc.want {
				t.Fatalf("decoded session mismatch")
			}
		})
	}
}

func TestWebSocketRejectsUnvalidatedSession(t *testing.T) {
	server := setupTestServer(t)
	defer server.scheduler.Stop()
	server.config.Security.EnableAuth = true
	server.config.Security.APIKey = "configured-key"
	req := httptest.NewRequest("GET", "/ws", nil)
	req.Header.Set("Sec-WebSocket-Protocol", "transiva-session."+base64.RawURLEncoding.EncodeToString([]byte("unknown-session")))
	w := httptest.NewRecorder()
	server.handleWebSocket(w, req)
	if w.Code != 401 {
		t.Fatalf("unvalidated session: expected 401, got %d", w.Code)
	}
}
