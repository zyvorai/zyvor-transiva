// Copyright 2026 Zyvor AI Labs · https://zyvor.dev
// SPDX-License-Identifier: Apache-2.0

package api

import (
	"net/http"
	"net/http/httptest"
	"testing"
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
