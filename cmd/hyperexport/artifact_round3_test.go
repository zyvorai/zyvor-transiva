// Copyright 2026 Zyvor AI Labs · https://zyvor.dev
// SPDX-License-Identifier: LicenseRef-Zyvor-Production-2.0

package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func resetRound3Flags() {
	*reportStraysFlag = false
	*assessFlag, *assessFailOnFlag = "", "error"
	*estimateFlag, *estimateTarget, *estimateFreeDir, *estimateFreeBytes = "", "qcow2", "", -1
	*fillChecksumsFlag = ""
}

func TestRunArtifactCommandsRound3(t *testing.T) {
	defer resetArtifactFlags()
	defer resetRound3Flags()

	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "d.img"), []byte("0123456789"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "extra.vmdk"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	mp := filepath.Join(dir, "m.json")
	m := `{"manifest_version":"1.0","vm":{"firmware":"bios","os_hint":"linux","cpu":1,"mem_gb":1},"disks":[{"id":"d0","source_format":"raw","bytes":10,"local_path":"d.img"}]}`
	if err := os.WriteFile(mp, []byte(m), 0o600); err != nil {
		t.Fatal(err)
	}

	run := func(setup func()) (int, string, string) {
		resetArtifactFlags()
		resetRound3Flags()
		setup()
		var out, errb bytes.Buffer
		handled, code := runArtifactCommands(&out, &errb)
		if !handled {
			t.Fatal("expected handled")
		}
		return code, out.String(), errb.String()
	}

	// assess: warning only (no checksum) -> conditionally_ready, exit 0 by default, 1 with warning gate
	code, out, _ := run(func() { *assessFlag = mp })
	if code != 0 || !strings.Contains(out, "conditionally_ready") || !strings.Contains(out, "no_checksum") {
		t.Fatalf("assess: %d %s", code, out)
	}
	if code, _, _ := run(func() { *assessFlag, *assessFailOnFlag = mp, "warning" }); code != 1 {
		t.Fatalf("assess fail-on warning: %d", code)
	}
	if code, _, e := run(func() { *assessFlag, *assessFailOnFlag = mp, "bogus" }); code != 2 || !strings.Contains(e, "assess-fail-on") {
		t.Fatalf("bad fail-on: %d %s", code, e)
	}
	if code, _, _ := run(func() { *assessFlag = filepath.Join(dir, "nope.json") }); code != 2 {
		t.Fatalf("missing manifest: %d", code)
	}
	code, out, _ = run(func() { *assessFlag, *jsonOutFlag = mp, true })
	var ar map[string]any
	if code != 0 || json.Unmarshal([]byte(out), &ar) != nil || ar["readiness"] != "conditionally_ready" {
		t.Fatalf("assess json: %d %s", code, out)
	}
	bad := filepath.Join(dir, "bad.json")
	_ = os.WriteFile(bad, []byte(`{"manifest_version":"1.0","disks":[{"id":"d0","source_format":"raw","bytes":10,"local_path":"d.img"}],"nics":[{"mac":"00:11:22:33:44:55"},{"mac":"00:11:22:33:44:55"}]}`), 0o600)
	if code, out, _ := run(func() { *assessFlag = bad }); code != 1 || !strings.Contains(out, "not_ready") {
		t.Fatalf("assess not ready: %d %s", code, out)
	}

	// estimate-space
	code, out, _ = run(func() { *estimateFlag, *estimateFreeBytes, *jsonOutFlag = mp, 1<<20, true })
	var er map[string]any
	if code != 0 || json.Unmarshal([]byte(out), &er) != nil || er["fit"] != "fits" {
		t.Fatalf("estimate: %d %s", code, out)
	}
	if code, out, _ := run(func() { *estimateFlag, *estimateFreeBytes = mp, 5 }); code != 1 || !strings.Contains(out, "does_not_fit") {
		t.Fatalf("estimate no fit: %d %s", code, out)
	}
	if code, out, _ := run(func() { *estimateFlag, *estimateFreeDir = mp, dir }); code != 0 || !strings.Contains(out, "fit: fits") {
		t.Fatalf("estimate free dir: %d %s", code, out)
	}
	if code, out, _ := run(func() { *estimateFlag = mp }); code != 0 || !strings.Contains(out, "fit: unknown") {
		t.Fatalf("estimate unknown: %d %s", code, out)
	}
	if code, _, _ := run(func() { *estimateFlag, *estimateTarget = mp, "vhdx" }); code != 2 {
		t.Fatalf("estimate bad target: %d", code)
	}
	if code, _, _ := run(func() { *estimateFlag, *estimateFreeDir = mp, filepath.Join(dir, "nodir") }); code != 2 {
		t.Fatalf("estimate bad dir: %d", code)
	}

	// report-strays on verify-bundle: warning only, still PASS
	code, out, _ = run(func() { *verifyBundleFlag, *reportStraysFlag, *skipChecksumsFlag = mp, true, true })
	if code != 0 || !strings.Contains(out, "stray_disk_file [extra.vmdk]") || !strings.Contains(out, "PASS") {
		t.Fatalf("strays: %d %s", code, out)
	}
	_, out, _ = run(func() { *verifyBundleFlag, *skipChecksumsFlag = mp, true })
	if strings.Contains(out, "stray") {
		t.Fatalf("strays must be opt-in: %s", out)
	}

	// fill-checksums writes to -manifest-out, then refuses to overwrite it
	outp := filepath.Join(dir, "filled.json")
	if code, out, e := run(func() { *fillChecksumsFlag, *manifestOutFlag = mp, outp }); code != 0 || !strings.Contains(out, "checksums added for: d0") {
		t.Fatalf("fill: %d %s %s", code, out, e)
	}
	if code, _, _ := run(func() { *fillChecksumsFlag, *manifestOutFlag = mp, outp }); code != 2 {
		t.Fatalf("fill must not overwrite: %d", code)
	}
	if code, out, _ := run(func() { *verifyBundleFlag, *requireChecksums = outp, true }); code != 0 || !strings.Contains(out, "checksum OK") {
		t.Fatalf("filled manifest must verify with checksums required: %d %s", code, out)
	}
	// in-place, json output; unreadable disk gives exit 1
	ip := filepath.Join(dir, "inplace.json")
	_ = os.WriteFile(ip, []byte(`{"manifest_version":"1.0","disks":[{"id":"d0","source_format":"raw","bytes":10,"local_path":"d.img"},{"id":"d1","source_format":"raw","bytes":10,"local_path":"gone.img"}]}`), 0o600)
	code, out, _ = run(func() { *fillChecksumsFlag, *jsonOutFlag = ip, true })
	var fr map[string]any
	if code != 1 || json.Unmarshal([]byte(out), &fr) != nil || fr["written"] != true {
		t.Fatalf("fill in place: %d %s", code, out)
	}
	if code, _, _ := run(func() { *fillChecksumsFlag = filepath.Join(dir, "nope.json") }); code != 2 {
		t.Fatalf("fill missing manifest: %d", code)
	}
}
