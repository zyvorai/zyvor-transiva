// Copyright 2026 Zyvor AI Labs · https://zyvor.dev
// SPDX-License-Identifier: Apache-2.0

package main

import "testing"

func TestNeedsVCenterEnv(t *testing.T) {
	for provider, want := range map[string]bool{
		"vsphere": true, "": true, "aws": true, "nutanix": false, "Nutanix": false,
	} {
		if got := needsVCenterEnv(provider); got != want {
			t.Errorf("needsVCenterEnv(%q) = %v, want %v", provider, got, want)
		}
	}
}
