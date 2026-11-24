// Copyright 2026 Zyvor AI Labs · https://zyvor.dev
// SPDX-License-Identifier: LicenseRef-Zyvor-Production-2.0

package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAssessAndEstimateHandlers(t *testing.T) {
	t.Setenv("ARTIFACT_ROOT", "")
	root := t.TempDir()
	outside := t.TempDir()
	_ = os.WriteFile(filepath.Join(root, "d.img"), make([]byte, 100), 0o600)
	_ = os.WriteFile(filepath.Join(root, "stray.vmdk"), []byte("x"), 0o600)
	m := `{"manifest_version":"1.0","disks":[{"id":"d0","source_format":"raw","bytes":6400,"local_path":"d.img"}]}`
	_ = os.WriteFile(filepath.Join(root, "m.json"), []byte(m), 0o600)
	_ = os.WriteFile(filepath.Join(root, "bad.json"), []byte(`{"manifest_version":"1.0","disks":[]}`), 0o600)
	_ = os.WriteFile(filepath.Join(outside, "m.json"), []byte(m), 0o600)
	_ = os.Symlink(outside, filepath.Join(root, "escape"))
	s := newArtifactServer(root)

	tests := []struct {
		name     string
		h        http.HandlerFunc
		body     string
		wantCode int
		contains string
	}{
		{"assess ok", s.handleAssessManifest, `{"manifest_path":"m.json"}`, 200, `"readiness":"conditionally_ready"`},
		{"assess missing path", s.handleAssessManifest, `{}`, 400, "required"},
		{"assess unknown field", s.handleAssessManifest, `{"manifest_path":"m.json","x":1}`, 400, "invalid request"},
		{"assess traversal", s.handleAssessManifest, `{"manifest_path":"../x.json"}`, 400, "inside the artifact root"},
		{"assess symlink escape", s.handleAssessManifest, `{"manifest_path":"escape/m.json"}`, 400, "inside the artifact root"},
		{"assess invalid manifest", s.handleAssessManifest, `{"manifest_path":"bad.json"}`, 422, "could not be assessed"},
		{"estimate unknown free", s.handleEstimateSpace, `{"manifest_path":"m.json"}`, 200, `"fit":"unknown"`},
		{"estimate fits", s.handleEstimateSpace, `{"manifest_path":"m.json","free_bytes":1000000}`, 200, `"fit":"fits"`},
		{"estimate does not fit", s.handleEstimateSpace, `{"manifest_path":"m.json","free_bytes":10}`, 200, `"fit":"does_not_fit"`},
		{"estimate raw", s.handleEstimateSpace, `{"manifest_path":"m.json","target_format":"raw"}`, 200, `"min_total_bytes":6400`},
		{"estimate bad format", s.handleEstimateSpace, `{"manifest_path":"m.json","target_format":"vhdx"}`, 400, "qcow2 or raw"},
		{"estimate negative free", s.handleEstimateSpace, `{"manifest_path":"m.json","free_bytes":-5}`, 400, "negative"},
		{"estimate traversal", s.handleEstimateSpace, `{"manifest_path":"escape/m.json"}`, 400, "inside the artifact root"},
		{"estimate invalid manifest", s.handleEstimateSpace, `{"manifest_path":"bad.json"}`, 422, "could not be read"},
		{"verify strays", s.handleVerifyBundle, `{"manifest_path":"m.json","skip_checksums":true,"report_strays":true}`, 200, `"stray_disk_file"`},
		{"verify no strays by default", s.handleVerifyBundle, `{"manifest_path":"m.json","skip_checksums":true}`, 200, `"findings":[`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := post(tt.h, tt.body)
			if rec.Code != tt.wantCode || !strings.Contains(rec.Body.String(), tt.contains) {
				t.Fatalf("code=%d body=%s", rec.Code, rec.Body.String())
			}
			if strings.Contains(rec.Body.String(), outside) {
				t.Fatal("response must not leak locations outside the root")
			}
			if tt.name == "verify no strays by default" && strings.Contains(rec.Body.String(), "stray") {
				t.Fatal("strays must be opt-in")
			}
		})
	}

	// estimate response carries the disclaimer and a parseable shape
	var est map[string]any
	rec := post(s.handleEstimateSpace, `{"manifest_path":"m.json"}`)
	if err := json.Unmarshal(rec.Body.Bytes(), &est); err != nil || est["disclaimer"] == "" || est["target_format"] != "qcow2" {
		t.Fatalf("shape: %v %s", err, rec.Body.String())
	}
	for _, h := range []http.HandlerFunc{s.handleAssessManifest, s.handleEstimateSpace} {
		r := httptest.NewRecorder()
		h(r, httptest.NewRequest(http.MethodGet, "/x", nil))
		if r.Code != http.StatusMethodNotAllowed {
			t.Fatalf("GET must be 405, got %d", r.Code)
		}
		if r := post(newArtifactServer("").handleAssessManifest, `{"manifest_path":"m.json"}`); r.Code != http.StatusServiceUnavailable {
			t.Fatalf("disabled: %d", r.Code)
		}
	}
}
