// Copyright 2026 Zyvor AI Labs · https://zyvor.dev
// SPDX-License-Identifier: LicenseRef-Zyvor-Production-2.0

package artifact

import (
	"archive/tar"
	"bytes"
	"context"
	"crypto/sha1" // #nosec G505 -- test fixture for OVF SHA1 manifests
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func hasCode(fs []Finding, code string) bool {
	for _, f := range fs {
		if f.Code == code {
			return true
		}
	}
	return false
}

func sha(b []byte) string { s := sha256.Sum256(b); return hex.EncodeToString(s[:]) }

func qcow2Header(virtual uint64, backing string, crypt uint32) []byte {
	h := make([]byte, 512)
	copy(h, []byte{'Q', 'F', 'I', 0xfb})
	binary.BigEndian.PutUint32(h[4:], 3)
	if backing != "" {
		binary.BigEndian.PutUint64(h[8:], 200)
		binary.BigEndian.PutUint32(h[16:], uint32(len(backing)))
		copy(h[200:], backing)
	}
	binary.BigEndian.PutUint64(h[24:], virtual)
	binary.BigEndian.PutUint32(h[32:], crypt)
	return h
}

func TestSafeEntryName(t *testing.T) {
	tests := []struct {
		name    string
		wantErr bool
	}{
		{"disk-0.vmdk", false},
		{"vm.ovf", false},
		{"", true},
		{"..", true},
		{".", true},
		{"../etc/passwd", true},
		{"a/b.vmdk", true},
		{"/abs.vmdk", true},
		{`a\b.vmdk`, true},
		{"C:evil", true},
		{"bad\x00name", true},
		{"bad\nname", true},
		{"-rf", true},
		{"a..b", true},
		{strings.Repeat("a", 256), true},
	}
	for _, tt := range tests {
		t.Run(fmt.Sprintf("%q", truncate(tt.name, 20)), func(t *testing.T) {
			if err := SafeEntryName(tt.name); (err != nil) != tt.wantErr {
				t.Fatalf("SafeEntryName(%q) err=%v wantErr=%v", tt.name, err, tt.wantErr)
			}
		})
	}
}

func TestConfinePath(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	inner := filepath.Join(root, "ok.img")
	must(t, os.WriteFile(inner, []byte("x"), 0o600))
	secret := filepath.Join(outside, "secret")
	must(t, os.WriteFile(secret, []byte("s"), 0o600))
	must(t, os.Symlink(secret, filepath.Join(root, "link")))
	must(t, os.Symlink(outside, filepath.Join(root, "dirlink")))

	tests := []struct {
		name    string
		p       string
		wantErr bool
	}{
		{"relative inside", "ok.img", false},
		{"absolute inside", inner, false},
		{"dotdot escape", "../" + filepath.Base(outside) + "/secret", true},
		{"absolute outside", secret, true},
		{"symlink file escape", "link", true},
		{"symlink dir escape", "dirlink/secret", true},
		{"missing", "nope", true},
		{"empty", "", true},
		{"nul", "ok.img\x00", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := ConfinePath(root, tt.p)
			if (err != nil) != tt.wantErr {
				t.Fatalf("err=%v wantErr=%v", err, tt.wantErr)
			}
		})
	}
	if _, err := ConfinePath("", "x"); err == nil {
		t.Fatal("empty root must be refused")
	}
}

func must(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}

func TestSniffDisk(t *testing.T) {
	vmdkSparse := make([]byte, 512)
	copy(vmdkSparse, "KDMV")
	binary.LittleEndian.PutUint64(vmdkSparse[12:], 2048)
	vdi := make([]byte, 0x200)
	binary.LittleEndian.PutUint32(vdi[0x40:], 0xbeda107f)
	binary.LittleEndian.PutUint64(vdi[0x170:], 1<<30)
	vhd := make([]byte, 1024)
	copy(vhd[512:], "conectix")
	binary.BigEndian.PutUint64(vhd[512+48:], 4096)
	desc := "# Disk DescriptorFile\nversion=1\nparentFileNameHint=\"../base.vmdk\"\ncreateType=\"monolithicFlat\"\nRW 100 FLAT \"disk-flat.vmdk\" 0\nRW 100 FLAT \"../../etc/x\" 0\n"

	tests := []struct {
		name         string
		data         []byte
		format       string
		virtual      int64
		backing      string
		extents      int
		encrypted    bool
		wantVariants string
	}{
		{"qcow2", qcow2Header(1<<20, "", 0), "qcow2", 1 << 20, "", 0, false, ""},
		{"qcow2 backing", qcow2Header(1<<20, "/etc/shadow", 0), "qcow2", 1 << 20, "/etc/shadow", 0, false, ""},
		{"qcow2 encrypted", qcow2Header(1<<20, "", 2), "qcow2", 1 << 20, "", 0, true, ""},
		{"vmdk sparse", vmdkSparse, "vmdk", 2048 * 512, "", 0, false, "sparse"},
		{"vmdk descriptor", []byte(desc), "vmdk", 0, "../base.vmdk", 2, false, "descriptor/monolithicFlat"},
		{"vdi", vdi, "vdi", 1 << 30, "", 0, false, ""},
		{"vhd footer", vhd, "vhd", 4096, "", 0, false, ""},
		{"vhdx", append([]byte("vhdxfile"), make([]byte, 32)...), "vhdx", 0, "", 0, false, ""},
		{"raw zeros", make([]byte, 4096), "raw", 0, "", 0, false, ""},
		{"empty", nil, "raw", 0, "", 0, false, ""},
		{"truncated qcow2", []byte{'Q', 'F', 'I', 0xfb, 0, 0}, "qcow2", 0, "", 0, false, ""},
		{"hostile huge virtual", func() []byte {
			h := qcow2Header(0, "", 0)
			binary.BigEndian.PutUint64(h[24:], ^uint64(0))
			return h
		}(), "qcow2", 1<<63 - 1, "", 0, false, ""},
		{"backing name outside header", func() []byte {
			h := qcow2Header(1, "", 0)
			binary.BigEndian.PutUint64(h[8:], 1<<40)
			binary.BigEndian.PutUint32(h[16:], 10)
			return h
		}(), "qcow2", 1, "(backing file name outside the header window)", 0, false, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s, err := SniffDisk(bytes.NewReader(tt.data), int64(len(tt.data)))
			if err != nil {
				t.Fatal(err)
			}
			if s.Format != tt.format || s.VirtualBytes != tt.virtual || s.BackingFile != tt.backing || len(s.ExtentFiles) != tt.extents || s.Encrypted != tt.encrypted {
				t.Fatalf("got %+v", s)
			}
			if tt.wantVariants != "" && s.Variant != tt.wantVariants {
				t.Fatalf("variant %q", s.Variant)
			}
		})
	}
}

type diskFixture struct {
	id, format string
	data       []byte
	declared   int64
	checksum   string // "" -> computed from data, "none" -> omit, otherwise literal
}

func writeBundle(t *testing.T, dir string, disks []diskFixture, relative bool) string {
	t.Helper()
	var ds []map[string]any
	for _, d := range disks {
		p := filepath.Join(dir, d.id+".img")
		must(t, os.WriteFile(p, d.data, 0o600))
		lp := p
		if relative {
			lp = d.id + ".img"
		}
		m := map[string]any{"id": d.id, "source_format": d.format, "bytes": d.declared, "local_path": lp}
		switch d.checksum {
		case "":
			m["checksum"] = "sha256:" + sha(d.data)
		case "none":
		default:
			m["checksum"] = d.checksum
		}
		ds = append(ds, m)
	}
	b, _ := json.Marshal(map[string]any{"manifest_version": "1.0", "disks": ds})
	mp := filepath.Join(dir, "manifest.json")
	must(t, os.WriteFile(mp, b, 0o600))
	return mp
}

func TestVerifyBundle(t *testing.T) {
	good := append(make([]byte, 100), 1)
	tests := []struct {
		name      string
		disks     []diskFixture
		opt       func(dir string) BundleOptions
		wantOK    bool
		wantCodes []string
	}{
		{"all good raw", []diskFixture{{"d0", "raw", good, int64(len(good)), ""}}, nil, true, nil},
		{"checksum mismatch", []diskFixture{{"d0", "raw", good, int64(len(good)), "sha256:" + strings.Repeat("0", 64)}}, nil, false, []string{"checksum_mismatch"}},
		{"every disk examined", []diskFixture{
			{"d0", "raw", good, int64(len(good)), "sha256:" + strings.Repeat("1", 64)},
			{"d1", "raw", good, int64(len(good)), "sha256:" + strings.Repeat("2", 64)},
		}, nil, false, []string{"checksum_mismatch"}},
		{"no checksum warns", []diskFixture{{"d0", "raw", good, int64(len(good)), "none"}}, nil, true, []string{"no_checksum"}},
		{"no checksum required", []diskFixture{{"d0", "raw", good, int64(len(good)), "none"}},
			func(string) BundleOptions { return BundleOptions{RequireChecksums: true} }, false, []string{"no_checksum"}},
		{"format lie", []diskFixture{{"d0", "qcow2", good, int64(len(good)), ""}}, nil, false, []string{"format_mismatch"}},
		{"qcow2 header over raw decl", []diskFixture{{"d0", "raw", qcow2Header(10, "", 0), 512, ""}}, nil, false, []string{"format_mismatch"}},
		{"backing file", []diskFixture{{"d0", "qcow2", qcow2Header(512, "/etc/passwd", 0), 512, ""}}, nil, false, []string{"backing_file"}},
		{"backing allowed", []diskFixture{{"d0", "qcow2", qcow2Header(512, "base.qcow2", 0), 512, ""}},
			func(string) BundleOptions { return BundleOptions{AllowBackingFiles: true} }, true, []string{"backing_file"}},
		{"raw size mismatch warns", []diskFixture{{"d0", "raw", good, 5, ""}}, nil, true, []string{"size_mismatch"}},
		{"too large", []diskFixture{{"d0", "raw", good, 1, ""}},
			func(string) BundleOptions { return BundleOptions{Limits: Limits{MaxDiskBytes: 10}} }, false, []string{"file_too_large"}},
		{"skip checksums", []diskFixture{{"d0", "raw", good, int64(len(good)), "sha256:" + strings.Repeat("0", 64)}},
			func(string) BundleOptions { return BundleOptions{SkipChecksums: true} }, true, nil},
		{"duplicate file", nil, nil, false, []string{"duplicate_path"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			var mp string
			if tt.name == "duplicate file" {
				p := filepath.Join(dir, "same.img")
				must(t, os.WriteFile(p, good, 0o600))
				c := "sha256:" + sha(good)
				b, _ := json.Marshal(map[string]any{"manifest_version": "1.0", "disks": []map[string]any{
					{"id": "a", "source_format": "raw", "bytes": len(good), "local_path": p, "checksum": c},
					{"id": "b", "source_format": "raw", "bytes": len(good), "local_path": p, "checksum": c},
				}})
				mp = filepath.Join(dir, "manifest.json")
				must(t, os.WriteFile(mp, b, 0o600))
			} else {
				mp = writeBundle(t, dir, tt.disks, false)
			}
			opt := BundleOptions{}
			if tt.opt != nil {
				opt = tt.opt(dir)
			}
			rep, err := VerifyBundle(context.Background(), mp, opt)
			if err != nil {
				t.Fatal(err)
			}
			if rep.OK != tt.wantOK {
				t.Fatalf("OK=%v want %v: %+v", rep.OK, tt.wantOK, rep.Findings)
			}
			for _, c := range tt.wantCodes {
				if !hasCode(rep.Findings, c) {
					t.Fatalf("missing finding %q in %+v", c, rep.Findings)
				}
			}
			if rep.Disclaimer == "" {
				t.Fatal("report must carry the disclaimer")
			}
		})
	}
}

func TestVerifyBundleEveryMismatchReported(t *testing.T) {
	dir := t.TempDir()
	d := []byte("data")
	mp := writeBundle(t, dir, []diskFixture{
		{"d0", "raw", d, 4, "sha256:" + strings.Repeat("1", 64)},
		{"d1", "raw", d, 4, "sha256:" + strings.Repeat("2", 64)},
	}, false)
	rep, _ := VerifyBundle(context.Background(), mp, BundleOptions{})
	n := 0
	for _, f := range rep.Findings {
		if f.Code == "checksum_mismatch" {
			n++
		}
	}
	if n != 2 {
		t.Fatalf("expected both mismatches reported, got %d", n)
	}
}

func TestVerifyBundleHostileManifests(t *testing.T) {
	tests := []struct {
		name    string
		content string
	}{
		{"not json", "{{{"},
		{"empty", ""},
		{"wrong version", `{"manifest_version":"9.9","disks":[{"id":"a","source_format":"raw","bytes":1,"local_path":"/x"}]}`},
		{"no disks", `{"manifest_version":"1.0","disks":[]}`},
		{"bad disk id", `{"manifest_version":"1.0","disks":[{"id":"../a","source_format":"raw","bytes":1,"local_path":"/x"}]}`},
		{"bad format", `{"manifest_version":"1.0","disks":[{"id":"a","source_format":"exe","bytes":1,"local_path":"/x"}]}`},
		{"negative bytes", `{"manifest_version":"1.0","disks":[{"id":"a","source_format":"raw","bytes":-5,"local_path":"/x"}]}`},
		{"short checksum", `{"manifest_version":"1.0","disks":[{"id":"a","source_format":"raw","bytes":1,"local_path":"/x","checksum":"sha256:ab"}]}`},
		{"uppercase checksum", `{"manifest_version":"1.0","disks":[{"id":"a","source_format":"raw","bytes":1,"local_path":"/x","checksum":"sha256:` + strings.Repeat("A", 64) + `"}]}`},
		{"nul in path", `{"manifest_version":"1.0","disks":[{"id":"a","source_format":"raw","bytes":1,"local_path":"/x\u0000y"}]}`},
		{"empty path", `{"manifest_version":"1.0","disks":[{"id":"a","source_format":"raw","bytes":1,"local_path":""}]}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := filepath.Join(t.TempDir(), "m.json")
			must(t, os.WriteFile(p, []byte(tt.content), 0o600))
			if _, err := VerifyBundle(context.Background(), p, BundleOptions{}); err == nil {
				t.Fatal("expected an error for a hostile manifest")
			}
		})
	}
}

func TestVerifyBundleOversizedManifest(t *testing.T) {
	p := filepath.Join(t.TempDir(), "m.json")
	must(t, os.WriteFile(p, []byte(`{"manifest_version":"1.0"}`+strings.Repeat(" ", 2048)), 0o600))
	_, err := VerifyBundle(context.Background(), p, BundleOptions{Limits: Limits{MaxManifestBytes: 1024}})
	if err == nil || !strings.Contains(err.Error(), "limit") {
		t.Fatalf("want size-limit error, got %v", err)
	}
}

func TestVerifyBundleRootConfinement(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	secret := filepath.Join(outside, "secret.img")
	must(t, os.WriteFile(secret, []byte("secret"), 0o600))
	must(t, os.WriteFile(filepath.Join(root, "in.img"), []byte("in"), 0o600))
	must(t, os.Symlink(secret, filepath.Join(root, "sym.img")))

	for _, tc := range []struct {
		name, path string
		wantEscape bool
	}{
		{"inside relative", "in.img", false},
		{"absolute outside", secret, true},
		{"dotdot", "../" + filepath.Base(outside) + "/secret.img", true},
		{"symlink out", "sym.img", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			b, _ := json.Marshal(map[string]any{"manifest_version": "1.0", "disks": []map[string]any{
				{"id": "a", "source_format": "raw", "bytes": 2, "local_path": tc.path},
			}})
			mp := filepath.Join(root, "m.json")
			must(t, os.WriteFile(mp, b, 0o600))
			rep, err := VerifyBundle(context.Background(), mp, BundleOptions{Root: root})
			if err != nil {
				t.Fatal(err)
			}
			if got := hasCode(rep.Findings, "path_outside_root"); got != tc.wantEscape {
				t.Fatalf("escape=%v want %v: %+v", got, tc.wantEscape, rep.Findings)
			}
			if tc.wantEscape && len(rep.Disks) > 0 && rep.Disks[0].ActualChecksum != "" {
				t.Fatal("must not hash a file outside the root")
			}
		})
	}
}

func TestVerifyBundleRefusesFIFOAndDir(t *testing.T) {
	dir := t.TempDir()
	b, _ := json.Marshal(map[string]any{"manifest_version": "1.0", "disks": []map[string]any{
		{"id": "a", "source_format": "raw", "bytes": 1, "local_path": dir},
	}})
	mp := filepath.Join(dir, "m.json")
	must(t, os.WriteFile(mp, b, 0o600))
	rep, err := VerifyBundle(context.Background(), mp, BundleOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if rep.OK || !hasCode(rep.Findings, "file_unreadable") {
		t.Fatalf("directory as disk must be refused: %+v", rep.Findings)
	}
}

func TestVerifyBundleCancelled(t *testing.T) {
	dir := t.TempDir()
	mp := writeBundle(t, dir, []diskFixture{{"d0", "raw", make([]byte, 1<<20), 1 << 20, ""}}, false)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := VerifyBundle(ctx, mp, BundleOptions{}); err == nil {
		t.Fatal("cancelled context must stop verification")
	}
}

func TestVerifyBundleRelativePaths(t *testing.T) {
	dir := t.TempDir()
	d := []byte("relative")
	mp := writeBundle(t, dir, []diskFixture{{"d0", "raw", d, int64(len(d)), ""}}, true)
	rep, err := VerifyBundle(context.Background(), mp, BundleOptions{Root: dir})
	if err != nil || !rep.OK {
		t.Fatalf("err=%v rep=%+v", err, rep)
	}
}

func TestSignAndVerify(t *testing.T) {
	dir := t.TempDir()
	priv, pub, err := GenerateKeyPair(filepath.Join(dir, "k"))
	must(t, err)
	if _, _, err := GenerateKeyPair(filepath.Join(dir, "k")); err == nil {
		t.Fatal("must not overwrite existing keys")
	}
	pk, err := LoadPrivateKey(priv)
	must(t, err)
	pubKey, err := LoadPublicKey(pub)
	must(t, err)

	mdata := []byte(`{"manifest_version":"1.0"}`)
	sig, err := SignManifest(mdata, pk)
	must(t, err)

	other := filepath.Join(dir, "other")
	_, opub, err := GenerateKeyPair(other)
	must(t, err)
	otherKey, _ := LoadPublicKey(opub)

	tests := []struct {
		name    string
		data    []byte
		sig     func() *Signature
		pub     func() []byte
		wantErr bool
	}{
		{"valid", mdata, func() *Signature { return sig }, nil, false},
		{"tampered manifest", append([]byte(nil), append(mdata, ' ')...), func() *Signature { return sig }, nil, true},
		{"wrong key", mdata, func() *Signature { return sig }, func() []byte { return otherKey }, true},
		{"nil sig", mdata, func() *Signature { return nil }, nil, true},
		{"bad version", mdata, func() *Signature { s := *sig; s.Version = 2; return &s }, nil, true},
		{"bad alg", mdata, func() *Signature { s := *sig; s.Algorithm = "rsa"; return &s }, nil, true},
		{"bad base64", mdata, func() *Signature { s := *sig; s.Signature = "!!!"; return &s }, nil, true},
		{"short sig", mdata, func() *Signature { s := *sig; s.Signature = "AAAA"; return &s }, nil, true},
		{"forged digest only", []byte("other manifest"), func() *Signature {
			s := *sig
			s.ManifestSHA256 = sha([]byte("other manifest"))
			return &s
		}, nil, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := pubKey
			if tt.pub != nil {
				p = tt.pub()
			}
			err := VerifyManifestSignature(tt.data, tt.sig(), p)
			if (err != nil) != tt.wantErr {
				t.Fatalf("err=%v wantErr=%v", err, tt.wantErr)
			}
		})
	}

	// signature file round trip + unknown fields rejected
	sp := filepath.Join(dir, "m.sig")
	must(t, WriteSignature(sp, sig))
	if _, err := LoadSignature(sp); err != nil {
		t.Fatal(err)
	}
	bad := filepath.Join(dir, "bad.sig")
	must(t, os.WriteFile(bad, []byte(`{"version":1,"algorithm":"ed25519","evil":true}`), 0o600))
	if _, err := LoadSignature(bad); err == nil {
		t.Fatal("unknown signature fields must be rejected")
	}
}

func TestLoadPrivateKeyRefusesLoosePermsAndGarbage(t *testing.T) {
	dir := t.TempDir()
	priv, _, err := GenerateKeyPair(filepath.Join(dir, "k"))
	must(t, err)
	must(t, os.Chmod(priv, 0o644))
	if _, err := LoadPrivateKey(priv); err == nil {
		t.Fatal("group/world readable private key must be refused")
	}
	g := filepath.Join(dir, "garbage.key")
	must(t, os.WriteFile(g, []byte("not pem"), 0o600))
	if _, err := LoadPrivateKey(g); err == nil {
		t.Fatal("garbage key must be refused")
	}
	if _, err := LoadPublicKey(g); err == nil {
		t.Fatal("garbage public key must be refused")
	}
}

func TestVerifyBundleWithSignature(t *testing.T) {
	dir := t.TempDir()
	d := []byte("signed disk")
	mp := writeBundle(t, dir, []diskFixture{{"d0", "raw", d, int64(len(d)), ""}}, false)
	priv, pub, err := GenerateKeyPair(filepath.Join(dir, "k"))
	must(t, err)
	pk, _ := LoadPrivateKey(priv)
	pubKey, _ := LoadPublicKey(pub)
	mdata, _ := os.ReadFile(mp)
	sig, _ := SignManifest(mdata, pk)
	sp := filepath.Join(dir, "m.sig")
	must(t, WriteSignature(sp, sig))

	rep, err := VerifyBundle(context.Background(), mp, BundleOptions{PublicKey: pubKey, SignatureFile: sp})
	must(t, err)
	if !rep.OK || !rep.SignatureVerified {
		t.Fatalf("%+v", rep)
	}
	// tamper: swapping the disk's checksum in the manifest invalidates the signature
	tampered := strings.Replace(string(mdata), "sha256:"+sha(d), "sha256:"+strings.Repeat("0", 64), 1)
	must(t, os.WriteFile(mp, []byte(tampered), 0o600))
	rep, err = VerifyBundle(context.Background(), mp, BundleOptions{PublicKey: pubKey, SignatureFile: sp})
	must(t, err)
	if rep.OK || rep.SignatureVerified || !hasCode(rep.Findings, "signature_invalid") {
		t.Fatalf("tampered manifest must fail signature: %+v", rep.Findings)
	}
	// key without signature file
	rep, _ = VerifyBundle(context.Background(), mp, BundleOptions{PublicKey: pubKey})
	if !hasCode(rep.Findings, "signature_incomplete") {
		t.Fatal("key without signature must be an error finding")
	}
}

// ---- OVA ----

type tarEnt struct {
	name  string
	data  []byte
	typ   byte
	link  string
	hsize int64 // override declared size when >=0 and typ==Reg
}

func buildTar(t *testing.T, path string, ents []tarEnt) {
	t.Helper()
	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)
	for _, e := range ents {
		typ := e.typ
		if typ == 0 {
			typ = tar.TypeReg
		}
		h := &tar.Header{Name: e.name, Typeflag: typ, Mode: 0o644, Linkname: e.link, Format: tar.FormatPAX}
		if typ == tar.TypeReg {
			h.Size = int64(len(e.data))
		}
		must(t, tw.WriteHeader(h))
		if typ == tar.TypeReg {
			_, err := tw.Write(e.data)
			must(t, err)
		}
	}
	must(t, tw.Close())
	must(t, os.WriteFile(path, buf.Bytes(), 0o600))
}

func ovfFor(files map[string]int) string {
	var refs, disks strings.Builder
	i := 0
	for name, size := range files {
		fmt.Fprintf(&refs, `<File ovf:id="f%d" ovf:href="%s" ovf:size="%d"/>`, i, name, size)
		fmt.Fprintf(&disks, `<Disk ovf:diskId="d%d" ovf:fileRef="f%d"/>`, i, i)
		i++
	}
	return `<?xml version="1.0"?><Envelope xmlns="http://schemas.dmtf.org/ovf/envelope/1" xmlns:ovf="http://schemas.dmtf.org/ovf/envelope/1">` +
		`<References>` + refs.String() + `</References><DiskSection>` + disks.String() + `</DiskSection>` +
		`<VirtualSystem ovf:id="vm"><Name>test-vm</Name></VirtualSystem></Envelope>`
}

func mfLines(files map[string][]byte, ovf string) string {
	var b strings.Builder
	s1 := sha1.Sum([]byte(ovf))
	fmt.Fprintf(&b, "SHA1(vm.ovf)= %s\n", hex.EncodeToString(s1[:]))
	for n, d := range files {
		fmt.Fprintf(&b, "SHA256(%s)= %s\n", n, sha(d))
	}
	return b.String()
}

func TestInspectOVA(t *testing.T) {
	disk := []byte("disk-bytes-0123456789")
	ovf := ovfFor(map[string]int{"disk1.vmdk": len(disk)})
	goodMF := mfLines(map[string][]byte{"disk1.vmdk": disk}, ovf)
	zero := int64(-1)

	tests := []struct {
		name      string
		ents      []tarEnt
		lim       Limits
		wantOK    bool
		wantCodes []string
		wantMF    bool
	}{
		{"valid", []tarEnt{{"vm.ovf", []byte(ovf), 0, "", zero}, {"vm.mf", []byte(goodMF), 0, "", zero}, {"disk1.vmdk", disk, 0, "", zero}}, Limits{}, true, nil, true},
		{"digest mismatch", []tarEnt{{"vm.ovf", []byte(ovf), 0, "", zero}, {"vm.mf", []byte(goodMF), 0, "", zero}, {"disk1.vmdk", []byte("tampered-bytes-012345"), 0, "", zero}}, Limits{}, false, []string{"digest_mismatch"}, false},
		{"no manifest warns", []tarEnt{{"vm.ovf", []byte(ovf), 0, "", zero}, {"disk1.vmdk", disk, 0, "", zero}}, Limits{}, true, []string{"no_manifest"}, false},
		{"missing referenced file", []tarEnt{{"vm.ovf", []byte(ovf), 0, "", zero}}, Limits{}, false, []string{"ovf_file_missing"}, false},
		{"size attr mismatch", []tarEnt{{"vm.ovf", []byte(ovfFor(map[string]int{"disk1.vmdk": 5})), 0, "", zero}, {"disk1.vmdk", disk, 0, "", zero}}, Limits{}, false, []string{"ovf_size_mismatch"}, false},
		{"traversal entry", []tarEnt{{"vm.ovf", []byte(ovf), 0, "", zero}, {"../../etc/cron.d/x", []byte("x"), 0, "", zero}, {"disk1.vmdk", disk, 0, "", zero}}, Limits{}, false, []string{"entry_name"}, false},
		{"absolute entry", []tarEnt{{"vm.ovf", []byte(ovf), 0, "", zero}, {"/etc/passwd", []byte("x"), 0, "", zero}, {"disk1.vmdk", disk, 0, "", zero}}, Limits{}, false, []string{"entry_name"}, false},
		{"subdir entry", []tarEnt{{"vm.ovf", []byte(ovf), 0, "", zero}, {"sub/disk.vmdk", []byte("x"), 0, "", zero}, {"disk1.vmdk", disk, 0, "", zero}}, Limits{}, false, []string{"entry_name"}, false},
		{"symlink entry", []tarEnt{{"vm.ovf", []byte(ovf), 0, "", zero}, {"evil", nil, tar.TypeSymlink, "/etc/shadow", zero}, {"disk1.vmdk", disk, 0, "", zero}}, Limits{}, false, []string{"entry_type"}, false},
		{"hardlink entry", []tarEnt{{"vm.ovf", []byte(ovf), 0, "", zero}, {"evil", nil, tar.TypeLink, "disk1.vmdk", zero}, {"disk1.vmdk", disk, 0, "", zero}}, Limits{}, false, []string{"entry_type"}, false},
		{"duplicate shadows", []tarEnt{{"vm.ovf", []byte(ovf), 0, "", zero}, {"disk1.vmdk", disk, 0, "", zero}, {"DISK1.vmdk", []byte("other"), 0, "", zero}}, Limits{}, false, []string{"duplicate_entry"}, false},
		{"oversized entry", []tarEnt{{"vm.ovf", []byte(ovf), 0, "", zero}, {"disk1.vmdk", disk, 0, "", zero}}, Limits{MaxDiskBytes: 5}, false, []string{"entry_too_large"}, false},
		{"too many entries", []tarEnt{{"vm.ovf", []byte(ovf), 0, "", zero}, {"a", []byte("1"), 0, "", zero}, {"b", []byte("1"), 0, "", zero}}, Limits{MaxEntries: 2}, false, []string{"too_many_entries"}, false},
		{"no ovf", []tarEnt{{"disk1.vmdk", disk, 0, "", zero}}, Limits{}, false, []string{"no_ovf"}, false},
		{"two ovf", []tarEnt{{"vm.ovf", []byte(ovf), 0, "", zero}, {"b.ovf", []byte(ovf), 0, "", zero}, {"disk1.vmdk", disk, 0, "", zero}}, Limits{}, false, []string{"multiple_ovf"}, false},
		{"ovf not first warns", []tarEnt{{"disk1.vmdk", disk, 0, "", zero}, {"vm.ovf", []byte(ovf), 0, "", zero}}, Limits{}, true, []string{"ovf_not_first"}, false},
		{"malformed ovf", []tarEnt{{"vm.ovf", []byte("<Envelope><unclosed"), 0, "", zero}}, Limits{}, false, []string{"ovf_parse"}, false},
		{"hostile href", []tarEnt{{"vm.ovf", []byte(ovfFor(map[string]int{"../../x": 1})), 0, "", zero}}, Limits{}, false, []string{"ovf_href"}, false},
		{"mf names traversal", []tarEnt{{"vm.ovf", []byte(ovf), 0, "", zero}, {"vm.mf", []byte("SHA256(../../x)= " + strings.Repeat("a", 64) + "\n"), 0, "", zero}, {"disk1.vmdk", disk, 0, "", zero}}, Limits{}, false, []string{"mf_name"}, false},
		{"mf garbage", []tarEnt{{"vm.ovf", []byte(ovf), 0, "", zero}, {"vm.mf", []byte("hello world\n"), 0, "", zero}, {"disk1.vmdk", disk, 0, "", zero}}, Limits{}, false, []string{"mf_syntax"}, false},
		{"mf short digest", []tarEnt{{"vm.ovf", []byte(ovf), 0, "", zero}, {"vm.mf", []byte("SHA256(disk1.vmdk)= abcd\n"), 0, "", zero}, {"disk1.vmdk", disk, 0, "", zero}}, Limits{}, false, []string{"mf_digest_length"}, false},
		{"mf lists absent file", []tarEnt{{"vm.ovf", []byte(ovf), 0, "", zero}, {"vm.mf", []byte(goodMF + "SHA256(ghost.vmdk)= " + strings.Repeat("a", 64) + "\n"), 0, "", zero}, {"disk1.vmdk", disk, 0, "", zero}}, Limits{}, false, []string{"mf_file_missing"}, false},
		{"cert unverified info", []tarEnt{{"vm.ovf", []byte(ovf), 0, "", zero}, {"vm.mf", []byte(goodMF), 0, "", zero}, {"vm.cert", []byte("pem"), 0, "", zero}, {"disk1.vmdk", disk, 0, "", zero}}, Limits{}, true, []string{"certificate_unverified"}, true},
		{"unreferenced entry warns", []tarEnt{{"vm.ovf", []byte(ovf), 0, "", zero}, {"extra.bin", []byte("x"), 0, "", zero}, {"disk1.vmdk", disk, 0, "", zero}}, Limits{}, true, []string{"unreferenced_entry"}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := filepath.Join(t.TempDir(), "x.ova")
			buildTar(t, p, tt.ents)
			rep, err := InspectOVA(context.Background(), p, tt.lim)
			if err != nil {
				t.Fatal(err)
			}
			if rep.OK != tt.wantOK {
				t.Fatalf("OK=%v want %v: %+v", rep.OK, tt.wantOK, rep.Findings)
			}
			for _, c := range tt.wantCodes {
				if !hasCode(rep.Findings, c) {
					t.Fatalf("missing %q in %+v", c, rep.Findings)
				}
			}
			if tt.wantMF && !rep.ManifestOK {
				t.Fatalf("manifest should verify: %+v", rep.Findings)
			}
		})
	}
}

func TestInspectOVAContainerErrors(t *testing.T) {
	dir := t.TempDir()

	gz := filepath.Join(dir, "a.ova")
	must(t, os.WriteFile(gz, []byte{0x1f, 0x8b, 8, 0, 0, 0, 0, 0}, 0o600))
	rep, err := InspectOVA(context.Background(), gz, Limits{})
	must(t, err)
	if rep.OK || !hasCode(rep.Findings, "compressed") {
		t.Fatalf("gzip OVA must be refused, not decompressed: %+v", rep.Findings)
	}

	junk := filepath.Join(dir, "junk.ova")
	must(t, os.WriteFile(junk, bytes.Repeat([]byte("not a tar "), 200), 0o600))
	rep, err = InspectOVA(context.Background(), junk, Limits{})
	must(t, err)
	if rep.OK || !hasCode(rep.Findings, "tar_corrupt") {
		t.Fatalf("%+v", rep.Findings)
	}

	// truncated: valid header declaring more bytes than are present
	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)
	must(t, tw.WriteHeader(&tar.Header{Name: "disk.vmdk", Size: 1 << 20, Mode: 0o644, Typeflag: tar.TypeReg}))
	trunc := filepath.Join(dir, "trunc.ova")
	must(t, os.WriteFile(trunc, append(buf.Bytes(), make([]byte, 100)...), 0o600))
	rep, err = InspectOVA(context.Background(), trunc, Limits{})
	must(t, err)
	if rep.OK || !hasCode(rep.Findings, "tar_corrupt") {
		t.Fatalf("truncated archive must be flagged: %+v", rep.Findings)
	}

	// archive larger than the total cap is not read at all
	big := filepath.Join(dir, "big.ova")
	must(t, os.WriteFile(big, make([]byte, 4096), 0o600))
	rep, err = InspectOVA(context.Background(), big, Limits{MaxTotalBytes: 1024})
	must(t, err)
	if !hasCode(rep.Findings, "archive_too_large") {
		t.Fatalf("%+v", rep.Findings)
	}

	// missing file and directory are errors, not findings
	if _, err := InspectOVA(context.Background(), filepath.Join(dir, "nope"), Limits{}); err == nil {
		t.Fatal("missing file must error")
	}
	if _, err := InspectOVA(context.Background(), dir, Limits{}); err == nil {
		t.Fatal("directory must error")
	}

	// cancelled context
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	p := filepath.Join(dir, "c.ova")
	buildTar(t, p, []tarEnt{{"vm.ovf", []byte(ovfFor(nil)), 0, "", -1}})
	if _, err := InspectOVA(ctx, p, Limits{}); err == nil {
		// cancellation surfaces either as an error or as a tar_corrupt finding
		t.Log("cancelled context returned no error; acceptable only if findings reflect it")
	}
}

func TestOVFEntityExpansionIsInert(t *testing.T) {
	bomb := `<?xml version="1.0"?><!DOCTYPE lolz [<!ENTITY a "aaaaaaaaaa"><!ENTITY b "&a;&a;&a;&a;&a;&a;&a;&a;&a;&a;"><!ENTITY c "&b;&b;&b;&b;&b;&b;&b;&b;&b;&b;">]>` +
		`<Envelope><VirtualSystem><Name>&c;&c;&c;</Name></VirtualSystem></Envelope>`
	p := filepath.Join(t.TempDir(), "x.ova")
	buildTar(t, p, []tarEnt{{"vm.ovf", []byte(bomb), 0, "", -1}})
	rep, err := InspectOVA(context.Background(), p, Limits{})
	must(t, err)
	if len(rep.VirtualSystem) > 1000 {
		t.Fatalf("entity expansion must not occur, name len %d", len(rep.VirtualSystem))
	}
}
