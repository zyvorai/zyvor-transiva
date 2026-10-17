// Copyright 2026 Zyvor AI Labs · https://zyvor.dev
// SPDX-License-Identifier: Apache-2.0

package main

import "testing"

func env(m map[string]string) func(string) string { return func(k string) string { return m[k] } }

func TestAuthIsOnByDefault(t *testing.T) {
	enable, key := authSettings(false, env(nil))
	if !enable || key != "" {
		t.Fatalf("authSettings = %v, %q; want auth on with no API key", enable, key)
	}
}

func TestAuthCanOnlyBeDisabledExplicitly(t *testing.T) {
	if enable, _ := authSettings(true, env(nil)); enable {
		t.Error("--no-auth should disable auth")
	}
	if enable, _ := authSettings(false, env(map[string]string{"TRANSIVA_DISABLE_AUTH": "1"})); enable {
		t.Error("TRANSIVA_DISABLE_AUTH=1 should disable auth")
	}
	// Anything else, including "true" or "0", keeps it on.
	for _, v := range []string{"", "0", "true", "yes"} {
		if enable, _ := authSettings(false, env(map[string]string{"TRANSIVA_DISABLE_AUTH": v})); !enable {
			t.Errorf("TRANSIVA_DISABLE_AUTH=%q must not disable auth", v)
		}
	}
}

func TestAPIKeyComesFromTheEnvironment(t *testing.T) {
	enable, key := authSettings(false, env(map[string]string{"TRANSIVA_API_KEY": "s3cret"}))
	if !enable || key != "s3cret" {
		t.Fatalf("authSettings = %v, %q; want auth on with the key", enable, key)
	}
}
