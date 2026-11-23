// Copyright 2026 Zyvor AI Labs · https://zyvor.dev
// SPDX-License-Identifier: LicenseRef-Zyvor-Production-2.0

package api

import (
	"bytes"
	"net/http"
	"testing"
)

func TestWorkflowManifestHandlersCapBodySize(t *testing.T) {
	s := newArtifactServer("")
	huge := `{"manifest_version":"1.0","pad":"` + string(bytes.Repeat([]byte("a"), 9<<20)) + `"}`
	for name, h := range map[string]http.HandlerFunc{
		"validate": s.ManifestValidateHandler,
		"submit":   s.ManifestSubmitHandler,
	} {
		t.Run(name, func(t *testing.T) {
			rec := post(h, huge)
			if rec.Code != http.StatusBadRequest {
				t.Fatalf("oversized body must be rejected with 400, got %d", rec.Code)
			}
		})
	}
}
