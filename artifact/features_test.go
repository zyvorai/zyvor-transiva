// Copyright 2026 Zyvor AI Labs · https://zyvor.dev
// SPDX-License-Identifier: LicenseRef-Zyvor-Production-2.0

package artifact

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/zyvorai/transiva/manifest"
)

func codes(fs []Finding) map[string]string {
	m := map[string]string{}
	for _, f := range fs {
		m[f.Code] = f.Severity
	}
	return m
}

func TestAssessManifest(t *testing.T) {
	good := &manifest.ArtifactManifest{
		ManifestVersion: "1.0",
		VM:              &manifest.VMMetadata{CPU: 2, MemGB: 4, Firmware: "bios", OSHint: "linux"},
		Disks:           []manifest.DiskArtifact{{ID: "d0", SourceFormat: "qcow2", Bytes: 1 << 30, LocalPath: "a", Checksum: "sha256:" + strings.Repeat("a", 64)}},
		NICs:            []manifest.NICInfo{{ID: "n0", MAC: "00:11:22:33:44:55"}},
	}
	r := AssessManifest(good)
	if r.Readiness != ReadinessReady || len(r.Findings) != 0 {
		t.Fatalf("clean manifest: %+v", r)
	}

	bad := &manifest.ArtifactManifest{
		ManifestVersion: "1.0",
		Source:          &manifest.SourceMetadata{Provider: "vsphere", VMName: "My VM_01"},
		VM:              &manifest.VMMetadata{Firmware: "uefi", SecureBoot: true, OSHint: "windows"},
		Disks: []manifest.DiskArtifact{
			{ID: "d0", SourceFormat: "vmdk", Bytes: 3 << 40, LocalPath: "a"},
			{ID: "d1", SourceFormat: "raw", Bytes: 17 << 40, LocalPath: "b"},
		},
		NICs:     []manifest.NICInfo{{ID: "n0", MAC: "00:11:22:33:44:55"}, {ID: "n1", MAC: "00-11-22-33-44-55"}, {ID: "n2", MAC: "zz"}, {ID: "n3"}},
		Warnings: []manifest.Warning{{Stage: "export", Message: "snapshot present"}},
	}
	r = AssessManifest(bad)
	c := codes(r.Findings)
	want := map[string]string{
		"uefi_firmware": SeverityInfo, "secure_boot": SeverityWarning, "windows_guest": SeverityWarning,
		"vm_name_not_dns1123": SeverityInfo, "multiple_disks": SeverityInfo, "no_boot_disk": SeverityWarning,
		"needs_conversion": SeverityInfo, "disk_over_2tib": SeverityWarning, "disk_too_large": SeverityError,
		"no_checksum": SeverityWarning, "nic_duplicate_mac": SeverityError, "nic_bad_mac": SeverityWarning,
		"nic_no_mac": SeverityInfo, "export_warning": SeverityWarning, "no_sizing": SeverityInfo,
	}
	for code, sev := range want {
		if c[code] != sev {
			t.Errorf("%s: got severity %q, want %q", code, c[code], sev)
		}
	}
	if r.Readiness != ReadinessNotReady || r.Errors != 2 || r.Warnings == 0 || r.Infos == 0 {
		t.Fatalf("counts: %+v", r)
	}

	warnOnly := &manifest.ArtifactManifest{ManifestVersion: "1.0", Disks: []manifest.DiskArtifact{{ID: "d", SourceFormat: "raw", Bytes: 1, LocalPath: "x"}}}
	if r := AssessManifest(warnOnly); r.Readiness != ReadinessConditional || codes(r.Findings)["no_vm_metadata"] != SeverityWarning {
		t.Fatalf("warn only: %+v", r)
	}
	if r.Disclaimer == "" {
		t.Fatal("disclaimer missing")
	}
}

func TestLoadManifestRejects(t *testing.T) {
	dir := t.TempDir()
	if _, err := LoadManifest(filepath.Join(dir, "nope.json"), Limits{}); err == nil {
		t.Fatal("missing file must fail")
	}
	p := filepath.Join(dir, "bad.json")
	_ = os.WriteFile(p, []byte("{"), 0o600)
	if _, err := LoadManifest(p, Limits{}); err == nil {
		t.Fatal("bad json must fail")
	}
	p = filepath.Join(dir, "empty.yaml")
	_ = os.WriteFile(p, []byte("manifest_version: \"1.0\"\ndisks: []\n"), 0o600)
	if _, err := LoadManifest(p, Limits{}); err == nil {
		t.Fatal("no disks must fail validation")
	}
	p = filepath.Join(dir, "big.json")
	_ = os.WriteFile(p, []byte(strings.Repeat(" ", 200)), 0o600)
	if _, err := LoadManifest(p, Limits{MaxManifestBytes: 100}); err == nil {
		t.Fatal("over the cap must fail")
	}
}

func TestEstimateSpace(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "a.img"), make([]byte, 1000), 0o600); err != nil {
		t.Fatal(err)
	}
	m := &manifest.ArtifactManifest{ManifestVersion: "1.0", Disks: []manifest.DiskArtifact{
		{ID: "d0", SourceFormat: "raw", Bytes: 64000, LocalPath: "a.img"},
		{ID: "d1", SourceFormat: "raw", Bytes: 6400, LocalPath: "missing.img"},
	}}
	e, err := EstimateSpace(m, EstimateOptions{FreeBytes: -1, ManifestDir: dir})
	if err != nil {
		t.Fatal(err)
	}
	if e.TargetFormat != "qcow2" || e.Fit != FitUnknown || e.FreeBytes != -1 {
		t.Fatalf("%+v", e)
	}
	d0, d1 := e.Disks[0], e.Disks[1]
	if d0.FileBytes != 1000 || d0.MinTargetBytes != 1000 || d0.MaxTargetBytes != 64000+1000 {
		t.Fatalf("d0 %+v", d0)
	}
	if d1.FileBytes != -1 || d1.MinTargetBytes != 6400 || d1.MaxTargetBytes != 6500 {
		t.Fatalf("d1 %+v", d1)
	}
	if codes(e.Findings)["file_missing"] != SeverityWarning {
		t.Fatalf("missing file not reported: %+v", e.Findings)
	}

	for _, tc := range []struct {
		free int64
		fit  string
	}{{1 << 20, FitYes}, {e.MaxTotalBytes, FitYes}, {e.MaxTotalBytes - 1, FitMaybe}, {e.MinTotalBytes, FitMaybe}, {e.MinTotalBytes - 1, FitNo}, {0, FitNo}} {
		got, _ := EstimateSpace(m, EstimateOptions{FreeBytes: tc.free, ManifestDir: dir})
		if got.Fit != tc.fit {
			t.Errorf("free=%d: fit %s, want %s", tc.free, got.Fit, tc.fit)
		}
	}

	raw, _ := EstimateSpace(m, EstimateOptions{TargetFormat: "raw", FreeBytes: -1, ManifestDir: dir})
	if raw.MinTotalBytes != 70400 || raw.MaxTotalBytes != 70400 {
		t.Fatalf("raw totals %+v", raw)
	}
	if _, err := EstimateSpace(m, EstimateOptions{TargetFormat: "vhdx", ManifestDir: dir}); err == nil {
		t.Fatal("unsupported target must fail")
	}

	// a file larger than its declared virtual size is capped
	_ = os.WriteFile(filepath.Join(dir, "big.img"), make([]byte, 500), 0o600)
	small := &manifest.ArtifactManifest{ManifestVersion: "1.0", Disks: []manifest.DiskArtifact{{ID: "s", SourceFormat: "raw", Bytes: 100, LocalPath: "big.img"}}}
	if g, _ := EstimateSpace(small, EstimateOptions{FreeBytes: -1, ManifestDir: dir}); g.Disks[0].MinTargetBytes != 100 {
		t.Fatalf("lower bound must be capped: %+v", g.Disks[0])
	}

	// confinement and overflow safety
	outside := t.TempDir()
	_ = os.WriteFile(filepath.Join(outside, "o.img"), []byte("x"), 0o600)
	esc := &manifest.ArtifactManifest{ManifestVersion: "1.0", Disks: []manifest.DiskArtifact{{ID: "e", SourceFormat: "raw", Bytes: 1, LocalPath: filepath.Join(outside, "o.img")}}}
	g, _ := EstimateSpace(esc, EstimateOptions{FreeBytes: -1, Root: dir, ManifestDir: dir})
	if codes(g.Findings)["path_outside_root"] != SeverityError || g.Disks[0].FileBytes != -1 {
		t.Fatalf("confinement: %+v", g)
	}
	huge := &manifest.ArtifactManifest{ManifestVersion: "1.0", Disks: []manifest.DiskArtifact{
		{ID: "h1", SourceFormat: "raw", Bytes: 1<<63 - 1, LocalPath: "x"}, {ID: "h2", SourceFormat: "raw", Bytes: 1<<63 - 1, LocalPath: "y"}}}
	if g, _ := EstimateSpace(huge, EstimateOptions{FreeBytes: -1, ManifestDir: dir}); g.MaxTotalBytes != 1<<63-1 || g.MaxTotalBytes < 0 {
		t.Fatalf("overflow: %d", g.MaxTotalBytes)
	}
}

func TestVerifyBundleStrays(t *testing.T) {
	dir := t.TempDir()
	disk := []byte("stray test disk")
	sum := sha256.Sum256(disk)
	_ = os.WriteFile(filepath.Join(dir, "d.img"), disk, 0o600)
	_ = os.WriteFile(filepath.Join(dir, "old.vmdk"), []byte("x"), 0o600)
	_ = os.WriteFile(filepath.Join(dir, "notes.txt"), []byte("x"), 0o600)
	_ = os.WriteFile(filepath.Join(dir, "SHA256SUMS"), []byte("x"), 0o600)
	_ = os.WriteFile(filepath.Join(dir, "m.json.sig"), []byte("x"), 0o600)
	_ = os.MkdirAll(filepath.Join(dir, "sub"), 0o700)
	_ = os.WriteFile(filepath.Join(dir, "sub", "left.qcow2"), []byte("x"), 0o600)
	_ = os.Symlink(filepath.Join(dir, "d.img"), filepath.Join(dir, "link.img"))
	mp := filepath.Join(dir, "m.json")
	_ = os.WriteFile(mp, []byte(`{"manifest_version":"1.0","disks":[{"id":"d0","source_format":"raw","bytes":15,"local_path":"d.img","checksum":"sha256:`+hex.EncodeToString(sum[:])+`"}]}`), 0o600)

	rep, err := VerifyBundle(context.Background(), mp, BundleOptions{})
	if err != nil || !rep.OK {
		t.Fatalf("baseline: %v %+v", err, rep)
	}
	for _, f := range rep.Findings {
		if strings.HasPrefix(f.Code, "stray") {
			t.Fatalf("strays must be off by default: %+v", f)
		}
	}

	rep, err = VerifyBundle(context.Background(), mp, BundleOptions{ReportStrays: true})
	if err != nil || !rep.OK {
		t.Fatalf("strays are warnings, not failures: %v %+v", err, rep)
	}
	got := map[string]string{}
	for _, f := range rep.Findings {
		got[f.Subject] = f.Code + "/" + f.Severity
	}
	want := map[string]string{
		"old.vmdk":                         "stray_disk_file/warning",
		filepath.Join("sub", "left.qcow2"): "stray_disk_file/warning",
		"notes.txt":                        "stray_file/info",
		"link.img":                         "stray_symlink/info",
	}
	for subj, w := range want {
		if got[subj] != w {
			t.Errorf("%s: got %q want %q (all: %v)", subj, got[subj], w, got)
		}
	}
	for _, quiet := range []string{"d.img", "m.json", "m.json.sig", "SHA256SUMS"} {
		if _, ok := got[quiet]; ok {
			t.Errorf("%s must not be reported", quiet)
		}
	}
}

func TestFillChecksums(t *testing.T) {
	dir := t.TempDir()
	a, b := []byte("disk a bytes"), []byte("disk b bytes")
	_ = os.WriteFile(filepath.Join(dir, "a.img"), a, 0o600)
	_ = os.WriteFile(filepath.Join(dir, "b.img"), b, 0o600)
	existing := "sha256:" + strings.Repeat("0", 64)
	mp := filepath.Join(dir, "m.json")
	_ = os.WriteFile(mp, []byte(`{"manifest_version":"1.0","disks":[
 {"id":"a","source_format":"raw","bytes":12,"local_path":"a.img"},
 {"id":"b","source_format":"raw","bytes":12,"local_path":"b.img","checksum":"`+existing+`"},
 {"id":"c","source_format":"raw","bytes":12,"local_path":"gone.img"}]}`), 0o640)
	_ = os.WriteFile(mp+".sig", []byte("old"), 0o600)

	res, err := FillChecksums(context.Background(), mp, FillOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if !res.Written || len(res.Filled) != 1 || res.Filled[0] != "a" || !res.SigStale {
		t.Fatalf("%+v", res)
	}
	c := codes(res.Findings)
	if c["file_unreadable"] != SeverityError || c["records_current_bytes"] != SeverityWarning || c["signature_stale"] != SeverityWarning {
		t.Fatalf("findings: %+v", res.Findings)
	}
	m, err := LoadManifest(mp, Limits{})
	if err != nil {
		t.Fatal(err)
	}
	sa := sha256.Sum256(a)
	if m.Disks[0].Checksum != "sha256:"+hex.EncodeToString(sa[:]) {
		t.Fatalf("a checksum %q", m.Disks[0].Checksum)
	}
	if m.Disks[1].Checksum != existing {
		t.Fatal("an existing checksum must never be replaced")
	}
	if m.Disks[2].Checksum != "" {
		t.Fatal("unreadable disk must stay without checksum")
	}
	if st, _ := os.Stat(mp); st.Mode().Perm() != 0o640 {
		t.Fatalf("mode changed: %v", st.Mode())
	}
	if left, _ := filepath.Glob(filepath.Join(dir, ".manifest-*.tmp")); len(left) != 0 {
		t.Fatalf("temp files left: %v", left)
	}

	// second run has nothing to fill for readable disks
	res, err = FillChecksums(context.Background(), mp, FillOptions{})
	if err != nil || res.Written || codes(res.Findings)["nothing_to_fill"] != SeverityInfo {
		t.Fatalf("idempotence: %v %+v", err, res)
	}

	// -out writes a new file and refuses to overwrite; the input is untouched
	m2 := filepath.Join(dir, "m2.yaml")
	_ = os.WriteFile(m2, []byte("manifest_version: \"1.0\"\ndisks:\n  - id: a\n    source_format: raw\n    bytes: 12\n    local_path: a.img\n"), 0o600)
	out := filepath.Join(dir, "out.yaml")
	res, err = FillChecksums(context.Background(), m2, FillOptions{OutPath: out})
	if err != nil || !res.Written || res.OutPath != out {
		t.Fatalf("out: %v %+v", err, res)
	}
	if got, err := LoadManifest(out, Limits{}); err != nil || got.Disks[0].Checksum == "" {
		t.Fatalf("yaml out: %v %+v", err, got)
	}
	if orig, _ := LoadManifest(m2, Limits{}); orig.Disks[0].Checksum != "" {
		t.Fatal("input must be untouched when an output path is given")
	}
	if _, err := FillChecksums(context.Background(), m2, FillOptions{OutPath: out}); err == nil {
		t.Fatal("must refuse to overwrite the output file")
	}

	// confinement
	outside := t.TempDir()
	_ = os.WriteFile(filepath.Join(outside, "x.img"), []byte("x"), 0o600)
	mo := filepath.Join(dir, "mo.json")
	_ = os.WriteFile(mo, []byte(`{"manifest_version":"1.0","disks":[{"id":"x","source_format":"raw","bytes":1,"local_path":"`+filepath.Join(outside, "x.img")+`"}]}`), 0o600)
	res, err = FillChecksums(context.Background(), mo, FillOptions{Root: dir})
	if err != nil || res.Written || codes(res.Findings)["path_outside_root"] != SeverityError {
		t.Fatalf("confine: %v %+v", err, res)
	}
}
