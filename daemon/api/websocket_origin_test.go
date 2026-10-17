// Copyright 2026 Zyvor AI Labs · https://zyvor.dev
// SPDX-License-Identifier: Apache-2.0

package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gorilla/websocket"
)

func TestIsSameOrigin(t *testing.T) {
	cases := []struct {
		origin, host string
		want         bool
	}{
		{"http://175.110.122.71:5080", "175.110.122.71:5080", true},
		{"https://dash.example.com", "dash.example.com", true},
		{"http://Dash.Example.com", "dash.example.com", true},
		{"http://175.110.122.71:5081", "175.110.122.71:5080", false}, // other port
		{"http://evil.example.com", "175.110.122.71:5080", false},
		{"http://175.110.122.71:5080.evil.com", "175.110.122.71:5080", false},
		{"null", "175.110.122.71:5080", false},
		{"", "175.110.122.71:5080", false},
	}
	for _, c := range cases {
		if got := isSameOrigin(c.origin, c.host); got != c.want {
			t.Errorf("isSameOrigin(%q, %q) = %v, want %v", c.origin, c.host, got, c.want)
		}
	}
}

// With sign-in required, the dashboard served by this daemon must be able to open its own WebSocket; an
// origin from somewhere else must still be refused.
func TestWebSocketUpgraderAllowsTheDashboardsOwnOrigin(t *testing.T) {
	server := setupTestServer(t)
	defer server.scheduler.Stop()
	server.config.Security.EnableAuth = true
	check := server.createUpgrader().CheckOrigin

	req := func(origin string) *http.Request {
		r := httptest.NewRequest(http.MethodGet, "http://175.110.122.71:5080/ws", nil)
		if origin != "" {
			r.Header.Set("Origin", origin)
		}
		return r
	}
	if !check(req("http://175.110.122.71:5080")) {
		t.Error("the dashboard's own origin must be allowed")
	}
	if !check(req("")) {
		t.Error("a client without an Origin header (not a browser) must be allowed")
	}
	if check(req("http://evil.example.com")) {
		t.Error("a foreign origin must be refused")
	}
}

// A browser drops a WebSocket whose server does not select one of the subprotocols it offered (close code 1006).
// The dashboard offers "transiva" and "transiva-session.<token>"; the server must select "transiva".
func TestWebSocketSelectsTheConstantSubprotocol(t *testing.T) {
	server := setupTestServer(t)
	defer server.scheduler.Stop()
	upgrader := server.createUpgrader()
	upgrader.CheckOrigin = func(*http.Request) bool { return true }

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		_ = conn.Close()
	}))
	defer srv.Close()

	dialer := websocket.Dialer{Subprotocols: []string{"transiva", "transiva-session.c2VjcmV0"}}
	conn, resp, err := dialer.Dial("ws"+strings.TrimPrefix(srv.URL, "http")+"/ws", nil)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer func() { _ = conn.Close() }()
	if got := resp.Header.Get("Sec-Websocket-Protocol"); got != "transiva" {
		t.Errorf("selected subprotocol = %q, want %q (and never the token)", got, "transiva")
	}
}
