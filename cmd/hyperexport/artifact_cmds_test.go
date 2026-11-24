// Copyright 2026 Zyvor AI Labs · https://zyvor.dev
// SPDX-License-Identifier: LicenseRef-Zyvor-Production-2.0

package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// resetArtifactFlags clears every artifact flag so cases are independent.
func resetArtifactFlags() {
	*verifyBundleFlag, *inspectOVAFlag, *signManifestFlag, *verifySigFlag = "", "", "", ""
	*genSigningKeyFlag, *signKeyFlag, *pubKeyFlag, *sigFileFlag, *bundleRootFlag = "", "", "", "", ""
	*requireChecksums, *allowBacking, *skipChecksumsFlag, *jsonOutFlag = false, false, false, false
	*maxDiskBytesFlag = 0
	*buildManifestFlag, *manifestOutFlag, *vmNameFlag, *diffManifestsFlag, *diffAgainstFlag, *writeSumsFlag = "", "", "", "", "", ""
	*diffPathsFlag = false
}

func TestRunArtifactCommands(t *testing.T) {
	defer resetArtifactFlags()
	resetArtifactFlags()

	if handled, _ := runArtifactCommands(&bytes.Buffer{}, &bytes.Buffer{}); handled {
		t.Fatal("no flag set must not be handled")
	}

	dir := t.TempDir()
	disk := []byte("cli disk bytes")
	sum := sha256.Sum256(disk)
	dp := filepath.Join(dir, "d.img")
	if err := os.WriteFile(dp, disk, 0o600); err != nil {
		t.Fatal(err)
	}
	mp := filepath.Join(dir, "m.json")
	m := fmt.Sprintf(`{"manifest_version":"1.0","disks":[{"id":"d0","source_format":"raw","bytes":%d,"local_path":%q,"checksum":"sha256:%s"}]}`, len(disk), dp, hex.EncodeToString(sum[:]))
	if err := os.WriteFile(mp, []byte(m), 0o600); err != nil {
		t.Fatal(err)
	}

	run := func(setup func()) (int, string, string) {
		resetArtifactFlags()
		setup()
		var out, errb bytes.Buffer
		handled, code := runArtifactCommands(&out, &errb)
		if !handled {
			t.Fatal("expected handled")
		}
		return code, out.String(), errb.String()
	}

	keyPrefix := filepath.Join(dir, "k")
	if code, _, e := run(func() { *genSigningKeyFlag = keyPrefix }); code != 0 {
		t.Fatalf("gen key: %d %s", code, e)
	}
	if code, _, _ := run(func() { *genSigningKeyFlag = keyPrefix }); code != 2 {
		t.Fatalf("regenerating over existing keys must fail with 2, got %d", code)
	}
	if code, _, e := run(func() { *signManifestFlag, *signKeyFlag = mp, keyPrefix+".key" }); code != 0 {
		t.Fatalf("sign: %d %s", code, e)
	}
	if code, out, _ := run(func() { *verifySigFlag, *pubKeyFlag = mp, keyPrefix+".pub" }); code != 0 || !strings.Contains(out, "OK") {
		t.Fatalf("verify sig: %d %s", code, out)
	}
	if code, out, e := run(func() { *verifyBundleFlag, *pubKeyFlag = mp, keyPrefix+".pub" }); code != 0 || !strings.Contains(out, "PASS") {
		t.Fatalf("verify bundle: %d %s %s", code, out, e)
	}

	code, out, _ := run(func() { *verifyBundleFlag, *jsonOutFlag = mp, true })
	var parsed map[string]any
	if code != 0 || json.Unmarshal([]byte(out), &parsed) != nil || parsed["ok"] != true {
		t.Fatalf("json output: %d %s", code, out)
	}

	// tamper with the disk: exit code 1, FAIL
	if err := os.WriteFile(dp, []byte("tampered"), 0o600); err != nil {
		t.Fatal(err)
	}
	if code, out, _ := run(func() { *verifyBundleFlag = mp }); code != 1 || !strings.Contains(out, "FAIL") {
		t.Fatalf("tampered disk: %d %s", code, out)
	}
	// tamper with the manifest: signature fails
	if err := os.WriteFile(mp, []byte(m+" "), 0o600); err != nil {
		t.Fatal(err)
	}
	if code, _, _ := run(func() { *verifySigFlag, *pubKeyFlag = mp, keyPrefix+".pub" }); code != 1 {
		t.Fatalf("tampered manifest signature must exit 1, got %d", code)
	}

	// usage errors
	if code, _, _ := run(func() { *signManifestFlag = mp }); code != 2 {
		t.Fatalf("sign without key: %d", code)
	}
	if code, _, _ := run(func() { *verifySigFlag = mp }); code != 2 {
		t.Fatalf("verify without pub: %d", code)
	}
	if code, _, _ := run(func() { *verifyBundleFlag = filepath.Join(dir, "missing.json") }); code != 2 {
		t.Fatalf("missing manifest: %d", code)
	}
	if code, _, _ := run(func() { *inspectOVAFlag = filepath.Join(dir, "missing.ova") }); code != 2 {
		t.Fatalf("missing ova: %d", code)
	}
}

func TestBuildDiffSumsCommands(t *testing.T) {
	defer resetArtifactFlags()
	dir := t.TempDir()
	disk := []byte("tool disk")
	if err := os.WriteFile(filepath.Join(dir, "a.img"), disk, 0o600); err != nil {
		t.Fatal(err)
	}
	run := func(setup func()) (int, string, string) {
		resetArtifactFlags()
		setup()
		var out, errb bytes.Buffer
		handled, code := runArtifactCommands(&out, &errb)
		if !handled {
			t.Fatal("expected handled")
		}
		return code, out.String(), errb.String()
	}
	mp := filepath.Join(dir, "artifact-manifest.json")
	if code, _, e := run(func() { *buildManifestFlag = dir }); code != 0 {
		t.Fatalf("build: %d %s", code, e)
	}
	if code, _, _ := run(func() { *buildManifestFlag = dir }); code != 2 {
		t.Fatalf("build must refuse to overwrite, got %d", code)
	}
	if code, out, e := run(func() { *verifyBundleFlag = mp }); code != 0 {
		t.Fatalf("generated manifest must verify: %d %s %s", code, out, e)
	}
	sum := sha256.Sum256(disk)
	if code, out, _ := run(func() { *writeSumsFlag = mp }); code != 0 || out != hex.EncodeToString(sum[:])+"  a.img\n" {
		t.Fatalf("sums: %d %q", code, out)
	}
	if code, out, _ := run(func() { *diffManifestsFlag, *diffAgainstFlag = mp, mp }); code != 0 || !strings.Contains(out, "equivalent") {
		t.Fatalf("self diff: %d %s", code, out)
	}
	if err := os.WriteFile(filepath.Join(dir, "a.img"), []byte("changed!!"), 0o600); err != nil {
		t.Fatal(err)
	}
	mp2 := filepath.Join(dir, "m2.json")
	if code, _, e := run(func() { *buildManifestFlag, *manifestOutFlag = dir, mp2 }); code != 0 {
		t.Fatalf("build2: %d %s", code, e)
	}
	if code, out, _ := run(func() { *diffManifestsFlag, *diffAgainstFlag = mp, mp2 }); code != 1 || !strings.Contains(out, "checksum") {
		t.Fatalf("diff: %d %s", code, out)
	}
	if code, _, _ := run(func() { *diffManifestsFlag = mp }); code != 2 {
		t.Fatalf("missing -diff-against must be 2, got %d", code)
	}
}
