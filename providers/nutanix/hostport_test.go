// Copyright 2026 Zyvor AI Labs · https://zyvor.dev
// SPDX-License-Identifier: Apache-2.0

package nutanix

import "testing"

func TestSplitHostPort(t *testing.T) {
	cases := []struct {
		name     string
		host     string
		port     int
		wantHost string
		wantPort int
	}{
		{"bare host", "prism.example.com", 0, "prism.example.com", 0},
		{"host and port", "prism.example.com:9441", 0, "prism.example.com", 9441},
		{"explicit port wins when the address has none", "prism.example.com", 9442, "prism.example.com", 9442},
		{"separate port kept over the one in the address", "prism.example.com:9441", 9442, "prism.example.com", 9442},
		{"https url with path", "https://prism.example.com:9441/console", 0, "prism.example.com", 9441},
		{"ip and non-default port", "127.0.0.1:8990", 0, "127.0.0.1", 8990},
		{"bracketed ipv6 with port", "[fd00::1]:9441", 0, "fd00::1", 9441},
		{"bare ipv6", "fd00::1", 0, "fd00::1", 0},
		{"whitespace is trimmed", "  prism:9441  ", 0, "prism", 9441},
		{"unparseable port is dropped", "prism:abc", 0, "prism", 0},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			host, port := SplitHostPort(c.host, c.port)
			if host != c.wantHost || port != c.wantPort {
				t.Errorf("SplitHostPort(%q, %d) = %q, %d; want %q, %d", c.host, c.port, host, port, c.wantHost, c.wantPort)
			}
		})
	}
}
