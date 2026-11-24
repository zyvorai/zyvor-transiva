// Copyright 2026 Zyvor AI Labs · https://zyvor.dev
// SPDX-License-Identifier: LicenseRef-Zyvor-Production-2.0

package artifact

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/zyvorai/transiva/manifest"
)

func writeFile(t *testing.T, p string, b []byte) {
	t.Helper()
	if err := os.WriteFile(p, b, 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestBuildManifestRoundTrip(t *testing.T) {
	dir := t.TempDir()
	q := append(qcow2Header(1<<30, "", 0), make([]byte, 100)...)
	writeFile(t, filepath.Join(dir, "boot disk.qcow2"), q)
	raw := []byte("raw disk contents")
	writeFile(t, filepath.Join(dir, "data.img"), raw)
	writeFile(t, filepath.Join(dir, "x-flat.vmdk"), []byte("no header here")) // flat extent: skipped
	writeFile(t, filepath.Join(dir, "notes.txt"), []byte("ignore"))
	if err := os.Mkdir(filepath.Join(dir, "sub.vmdk"), 0o700); err != nil {
		t.Fatal(err)
	}

	m, fs, err := BuildManifest(context.Background(), dir, BuildOptions{Provider: "vsphere", VMName: "vm1"})
	if err != nil {
		t.Fatal(err)
	}
	if len(m.Disks) != 2 {
		t.Fatalf("want 2 disks, got %+v", m.Disks)
	}
	if m.Disks[0].ID != "boot-disk" || m.Disks[0].SourceFormat != "qcow2" || m.Disks[0].Bytes != 1<<30 {
		t.Fatalf("disk0: %+v", m.Disks[0])
	}
	if m.Disks[1].SourceFormat != "raw" || m.Disks[1].Bytes != int64(len(raw)) || m.Disks[1].Checksum != "sha256:"+sha(raw) {
		t.Fatalf("disk1: %+v", m.Disks[1])
	}
	if !hasCode(fs, "file_skipped") {
		t.Fatalf("flat extent should be reported: %+v", fs)
	}

	// The generated manifest must verify cleanly against the same directory.
	data, err := manifest.ToJSON(m)
	if err != nil {
		t.Fatal(err)
	}
	mp := filepath.Join(dir, "artifact-manifest.json")
	writeFile(t, mp, data)
	rep, err := VerifyBundle(context.Background(), mp, BundleOptions{Root: dir})
	if err != nil {
		t.Fatal(err)
	}
	if !rep.OK {
		t.Fatalf("round trip failed: %+v", rep.Findings)
	}
}

func TestBuildManifestEdgeCases(t *testing.T) {
	if _, _, err := BuildManifest(context.Background(), t.TempDir(), BuildOptions{}); err == nil {
		t.Fatal("empty directory must error")
	}
	if _, _, err := BuildManifest(context.Background(), "/nonexistent-dir-xyz", BuildOptions{}); err == nil {
		t.Fatal("missing directory must error")
	}

	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "a.img"), []byte("a"))
	writeFile(t, filepath.Join(dir, "a.raw"), []byte("b")) // same stem: IDs must stay unique
	writeFile(t, filepath.Join(dir, "!!!.img"), []byte("c"))
	m, _, err := BuildManifest(context.Background(), dir, BuildOptions{SkipChecksums: true})
	if err != nil {
		t.Fatal(err)
	}
	seen := map[string]bool{}
	for _, d := range m.Disks {
		if seen[d.ID] {
			t.Fatalf("duplicate id %q", d.ID)
		}
		seen[d.ID] = true
		if d.Checksum != "" {
			t.Fatal("SkipChecksums must leave checksum empty")
		}
	}
	if len(m.Disks) != 3 {
		t.Fatalf("got %d disks", len(m.Disks))
	}

	// backing files are surfaced, not hidden
	d2 := t.TempDir()
	writeFile(t, filepath.Join(d2, "c.qcow2"), qcow2Header(4096, "base.qcow2", 0))
	_, fs, err := BuildManifest(context.Background(), d2, BuildOptions{})
	if err != nil || !hasCode(fs, "backing_file") {
		t.Fatalf("err=%v findings=%+v", err, fs)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, _, err := BuildManifest(ctx, dir, BuildOptions{}); err == nil {
		t.Fatal("cancelled context must error")
	}
}

func TestDiffManifests(t *testing.T) {
	a := &manifest.ArtifactManifest{ManifestVersion: "1.0", Disks: []manifest.DiskArtifact{
		{ID: "d0", SourceFormat: "raw", Bytes: 10, LocalPath: "/a/d0", Checksum: "sha256:" + strings.Repeat("a", 64)},
		{ID: "d1", SourceFormat: "raw", Bytes: 10, LocalPath: "/a/d1"},
	}}
	if d := DiffManifests(a, a, true); !d.Identical || len(d.Changes) != 0 {
		t.Fatalf("self diff: %+v", d)
	}
	b := &manifest.ArtifactManifest{ManifestVersion: "1.0",
		Source: &manifest.SourceMetadata{Provider: "vsphere"},
		Disks: []manifest.DiskArtifact{
			{ID: "d0", SourceFormat: "qcow2", Bytes: 10, LocalPath: "/b/d0", Checksum: "sha256:" + strings.Repeat("b", 64)},
			{ID: "d2", SourceFormat: "raw", Bytes: 5, LocalPath: "/b/d2"},
		}}
	d := DiffManifests(a, b, false)
	if d.Identical {
		t.Fatal("must differ")
	}
	got := map[string]bool{}
	for _, c := range d.Changes {
		got[c.Kind+"/"+c.Subject+"/"+c.Field] = true
	}
	for _, w := range []string{"changed/source/provider", "changed/d0/source_format", "changed/d0/checksum", "removed/d1/", "added/d2/"} {
		if !got[w] {
			t.Errorf("missing change %s in %+v", w, d.Changes)
		}
	}
	if got["changed/d0/local_path"] {
		t.Error("paths must be ignored unless requested")
	}
	if !contains(DiffManifests(a, b, true).Changes, "local_path") {
		t.Error("includePaths must report local_path")
	}
}

func contains(cs []Change, field string) bool {
	for _, c := range cs {
		if c.Field == field {
			return true
		}
	}
	return false
}

func TestWriteSHA256Sums(t *testing.T) {
	sum := strings.Repeat("AB", 32)
	m := &manifest.ArtifactManifest{Disks: []manifest.DiskArtifact{
		{ID: "d0", LocalPath: "/exp/vm/d0.img", Checksum: "sha256:" + sum},
		{ID: "d1", LocalPath: "rel/d1.img", Checksum: "sha256:" + strings.Repeat("0", 64)},
		{ID: "d2", LocalPath: "/other/d2.img"},
	}}
	var buf bytes.Buffer
	skipped, err := WriteSHA256Sums(&buf, m, "/exp/vm")
	if err != nil {
		t.Fatal(err)
	}
	want := strings.ToLower(sum) + "  d0.img\n" + strings.Repeat("0", 64) + "  rel/d1.img\n"
	if buf.String() != want {
		t.Fatalf("got %q want %q", buf.String(), want)
	}
	if len(skipped) != 1 || skipped[0] != "d2" {
		t.Fatalf("skipped %v", skipped)
	}

	for _, bad := range []manifest.DiskArtifact{
		{ID: "x", LocalPath: "a\nb", Checksum: "sha256:" + sum},
		{ID: "x", LocalPath: `a\b`, Checksum: "sha256:" + sum},
		{ID: "x", LocalPath: "a", Checksum: "sha256:abc"},
	} {
		if _, err := WriteSHA256Sums(&bytes.Buffer{}, &manifest.ArtifactManifest{Disks: []manifest.DiskArtifact{bad}}, "/"); err == nil {
			t.Errorf("expected error for %+v", bad)
		}
	}
}
