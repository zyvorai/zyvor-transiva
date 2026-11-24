// Copyright 2026 Zyvor AI Labs · https://zyvor.dev
// SPDX-License-Identifier: LicenseRef-Zyvor-Production-2.0

package openapi

import (
	"context"
	"os"
	"regexp"
	"testing"

	"github.com/getkin/kin-openapi/openapi3"
)

// TestRepoSpecDocumentsArtifactRoutes keeps openapi.yaml in sync with the
// /artifacts/* routes registered by the daemon: every route registered in
// daemon/api must be documented, and the spec must load.
func TestRepoSpecDocumentsArtifactRoutes(t *testing.T) {
	loader := openapi3.NewLoader()
	doc, err := loader.LoadFromFile("../../openapi.yaml")
	if err != nil {
		t.Fatalf("openapi.yaml does not load: %v", err)
	}
	if err := doc.Validate(context.Background()); err != nil {
		t.Fatalf("openapi.yaml is not valid: %v", err)
	}

	re := regexp.MustCompile(`HandleFunc\("(/artifacts/[a-z-]+)"`)
	seen := map[string]bool{}
	for _, f := range []string{"../api/server.go", "../api/enhanced_server.go"} {
		src, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		for _, m := range re.FindAllStringSubmatch(string(src), -1) {
			seen[m[1]] = true
			if doc.Paths.Find(m[1]) == nil {
				t.Errorf("route %s is registered in %s but missing from openapi.yaml", m[1], f)
			}
		}
	}
	want := []string{
		"/artifacts/verify-bundle", "/artifacts/inspect-ova", "/artifacts/verify-signature",
		"/artifacts/assess", "/artifacts/estimate-space",
	}
	for _, r := range want {
		if !seen[r] {
			t.Errorf("expected route %s to be registered in daemon/api", r)
		}
		if item := doc.Paths.Find(r); item == nil || item.Post == nil {
			t.Errorf("openapi.yaml must document POST %s", r)
		}
	}
	if len(seen) != len(want) {
		t.Fatalf("expected %d artifact routes registered, found %d: update this list and openapi.yaml together", len(want), len(seen))
	}
}
