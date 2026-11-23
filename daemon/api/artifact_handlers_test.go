// Copyright 2026 Zyvor AI Labs · https://zyvor.dev
// SPDX-License-Identifier: LicenseRef-Zyvor-Production-2.0

package api

import (
	"archive/tar"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/zyvorai/transiva/artifact"
	"github.com/zyvorai/transiva/config"
	"github.com/zyvorai/transiva/logger"
)

func newArtifactServer(root string) *Server {
	return &Server{logger: logger.New("info"), appConfig: &config.Config{ArtifactRoot: root}}
}

func post(h http.HandlerFunc, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, "/x", strings.NewReader(body))
	rec := httptest.NewRecorder()
	h(rec, req)
	return rec
}

func TestArtifactHandlers(t *testing.T) {
	t.Setenv("ARTIFACT_ROOT", "")
	root := t.TempDir()
	outside := t.TempDir()

	disk := []byte("api disk")
	sum := sha256.Sum256(disk)
	if err := os.WriteFile(filepath.Join(root, "d.img"), disk, 0o600); err != nil {
		t.Fatal(err)
	}
	m := fmt.Sprintf(`{"manifest_version":"1.0","disks":[{"id":"d0","source_format":"raw","bytes":%d,"local_path":"d.img","checksum":"sha256:%s"}]}`, len(disk), hex.EncodeToString(sum[:]))
	if err := os.WriteFile(filepath.Join(root, "m.json"), []byte(m), 0o600); err != nil {
		t.Fatal(err)
	}
	secretManifest := filepath.Join(outside, "m.json")
	if err := os.WriteFile(secretManifest, []byte(m), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(root, "escape")); err != nil {
		t.Fatal(err)
	}

	var ova bytes.Buffer
	tw := tar.NewWriter(&ova)
	ovf := `<Envelope xmlns:ovf="x"><References><File ovf:id="f" ovf:href="d.vmdk" ovf:size="3"/></References><VirtualSystem><Name>v</Name></VirtualSystem></Envelope>`
	for _, e := range []struct{ n, d string }{{"v.ovf", ovf}, {"d.vmdk", "abc"}} {
		if err := tw.WriteHeader(&tar.Header{Name: e.n, Size: int64(len(e.d)), Mode: 0o644, Typeflag: tar.TypeReg}); err != nil {
			t.Fatal(err)
		}
		_, _ = tw.Write([]byte(e.d))
	}
	_ = tw.Close()
	if err := os.WriteFile(filepath.Join(root, "v.ova"), ova.Bytes(), 0o600); err != nil {
		t.Fatal(err)
	}

	s := newArtifactServer(root)

	tests := []struct {
		name     string
		h        http.HandlerFunc
		body     string
		wantCode int
		contains string
	}{
		{"bundle ok", s.handleVerifyBundle, `{"manifest_path":"m.json"}`, 200, `"ok":true`},
		{"bundle traversal", s.handleVerifyBundle, `{"manifest_path":"../` + filepath.Base(outside) + `/m.json"}`, 400, "inside the artifact root"},
		{"bundle absolute outside", s.handleVerifyBundle, `{"manifest_path":` + jsonStr(secretManifest) + `}`, 400, "inside the artifact root"},
		{"bundle symlink escape", s.handleVerifyBundle, `{"manifest_path":"escape/m.json"}`, 400, "inside the artifact root"},
		{"bundle missing field", s.handleVerifyBundle, `{}`, 400, "required"},
		{"bundle unknown field", s.handleVerifyBundle, `{"manifest_path":"m.json","evil":1}`, 400, "invalid request"},
		{"bundle garbage body", s.handleVerifyBundle, `not json`, 400, "invalid request"},
		{"bundle oversized body", s.handleVerifyBundle, `{"manifest_path":"` + strings.Repeat("a", artifactMaxBody) + `"}`, 400, "invalid request"},
		{"bundle key without sig", s.handleVerifyBundle, `{"manifest_path":"m.json","public_key_path":"m.json"}`, 400, "signature_path is required"},
		{"ova ok", s.handleInspectOVA, `{"path":"v.ova"}`, 200, `"ok":true`},
		{"ova traversal", s.handleInspectOVA, `{"path":"../x.ova"}`, 400, "inside the artifact root"},
		{"sig missing key", s.handleVerifySignature, `{"manifest_path":"m.json","signature_path":"m.json"}`, 400, "public_key_path is required"},
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
		})
	}

	t.Run("method not allowed", func(t *testing.T) {
		rec := httptest.NewRecorder()
		s.handleVerifyBundle(rec, httptest.NewRequest(http.MethodGet, "/x", nil))
		if rec.Code != http.StatusMethodNotAllowed {
			t.Fatalf("code=%d", rec.Code)
		}
	})
	t.Run("disabled without root", func(t *testing.T) {
		rec := post(newArtifactServer("").handleVerifyBundle, `{"manifest_path":"m.json"}`)
		if rec.Code != http.StatusServiceUnavailable {
			t.Fatalf("code=%d", rec.Code)
		}
	})
	t.Run("busy returns 429", func(t *testing.T) {
		artifactSem <- struct{}{}
		artifactSem <- struct{}{}
		defer func() { <-artifactSem; <-artifactSem }()
		if rec := post(s.handleVerifyBundle, `{"manifest_path":"m.json"}`); rec.Code != http.StatusTooManyRequests {
			t.Fatalf("code=%d", rec.Code)
		}
	})
	t.Run("signature round trip", func(t *testing.T) {
		prefix := filepath.Join(root, "k")
		privP, pubP, err := artifact.GenerateKeyPair(prefix)
		if err != nil {
			t.Fatal(err)
		}
		priv, _ := artifact.LoadPrivateKey(privP)
		_ = pubP
		sig, _ := artifact.SignManifest([]byte(m), priv)
		if err := artifact.WriteSignature(filepath.Join(root, "m.sig"), sig); err != nil {
			t.Fatal(err)
		}
		rec := post(s.handleVerifySignature, `{"manifest_path":"m.json","signature_path":"m.sig","public_key_path":"k.pub"}`)
		var out map[string]any
		_ = json.Unmarshal(rec.Body.Bytes(), &out)
		if rec.Code != 200 || out["valid"] != true {
			t.Fatalf("%d %s", rec.Code, rec.Body.String())
		}
		rec = post(s.handleVerifyBundle, `{"manifest_path":"m.json","signature_path":"m.sig","public_key_path":"k.pub"}`)
		if rec.Code != 200 || !strings.Contains(rec.Body.String(), `"signature_verified":true`) {
			t.Fatalf("%d %s", rec.Code, rec.Body.String())
		}
		// edit manifest -> signature invalid
		_ = os.WriteFile(filepath.Join(root, "m.json"), []byte(m+" "), 0o600)
		rec = post(s.handleVerifySignature, `{"manifest_path":"m.json","signature_path":"m.sig","public_key_path":"k.pub"}`)
		if !strings.Contains(rec.Body.String(), `"valid":false`) {
			t.Fatalf("%s", rec.Body.String())
		}
	})
}

func jsonStr(s string) string { b, _ := json.Marshal(s); return string(b) }
