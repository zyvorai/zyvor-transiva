// Copyright 2026 Zyvor AI Labs · https://zyvor.dev
// SPDX-License-Identifier: LicenseRef-Zyvor-Production-2.0

package manifest

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestVerifyChecksums_MalformedAndAllMismatches(t *testing.T) {
	dir := t.TempDir()
	a := filepath.Join(dir, "a.img")
	b := filepath.Join(dir, "b.img")
	if err := os.WriteFile(a, []byte("aaa"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(b, []byte("bbb"), 0o600); err != nil {
		t.Fatal(err)
	}
	zero := "sha256:" + strings.Repeat("0", 64)

	tests := []struct {
		name      string
		disks     []DiskArtifact
		wantErr   string
		wantFalse []string
	}{
		{"short checksum does not panic", []DiskArtifact{{ID: "a", LocalPath: a, Checksum: "sha"}}, "malformed checksum", []string{"a"}},
		{"empty prefix only", []DiskArtifact{{ID: "a", LocalPath: a, Checksum: "sha256:"}}, "malformed checksum", []string{"a"}},
		{"uppercase hex rejected", []DiskArtifact{{ID: "a", LocalPath: a, Checksum: "sha256:" + strings.Repeat("A", 64)}}, "malformed checksum", []string{"a"}},
		{"both mismatches reported", []DiskArtifact{
			{ID: "a", LocalPath: a, Checksum: zero},
			{ID: "b", LocalPath: b, Checksum: zero},
		}, "disk \"b\"", []string{"a", "b"}},
		{"no checksum skipped", []DiskArtifact{{ID: "a", LocalPath: a}}, "", nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res, err := VerifyChecksums(&ArtifactManifest{Disks: tt.disks})
			if tt.wantErr == "" {
				if err != nil {
					t.Fatal(err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("err=%v want substring %q", err, tt.wantErr)
			}
			for _, id := range tt.wantFalse {
				if ok, present := res[id]; !present || ok {
					t.Fatalf("results[%q]=%v present=%v", id, ok, present)
				}
			}
		})
	}
}

func TestValidateSchemaDoesNotTouchFilesystem(t *testing.T) {
	m := &ArtifactManifest{ManifestVersion: CurrentVersion, Disks: []DiskArtifact{
		{ID: "d", SourceFormat: "raw", Bytes: 1, LocalPath: "/definitely/not/here.img"},
	}}
	if err := ValidateSchema(m); err != nil {
		t.Fatalf("ValidateSchema must not stat files: %v", err)
	}
	if err := Validate(m); err == nil {
		t.Fatal("Validate must still require the file")
	}
	m.Disks[0].LocalPath = "/x\x00y"
	if err := ValidateSchema(m); err == nil {
		t.Fatal("NUL in local_path must be rejected")
	}
}

func TestReadFromFile_RejectsOversizedAndSpecialFiles(t *testing.T) {
	dir := t.TempDir()
	big := filepath.Join(dir, "big.json")
	if err := os.WriteFile(big, make([]byte, MaxManifestFileBytes+1), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := ReadFromFile(big); err == nil || !strings.Contains(err.Error(), "limit") {
		t.Fatalf("oversized manifest must be refused, got %v", err)
	}
	if _, err := ReadFromFile(dir); err == nil {
		t.Fatal("directory must be refused")
	}
}
