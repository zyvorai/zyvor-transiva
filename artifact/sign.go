// Copyright 2026 Zyvor AI Labs · https://zyvor.dev
// SPDX-License-Identifier: LicenseRef-Zyvor-Production-2.0

package artifact

import (
	"bytes"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"os"
	"runtime"
)

// signaturePrefix domain-separates manifest signatures from any other use of
// the same key.
const signaturePrefix = "transiva-manifest-signature-v1\n"

// SignatureVersion is the detached-signature file format version.
const SignatureVersion = 1

// Signature is a detached Ed25519 signature over the exact bytes of a
// manifest file. It proves that the holder of the private key produced or
// approved those bytes (including every disk checksum they list). It says
// nothing about the disks' contents beyond that, and nothing about who the key
// holder is: trust in the public key is the verifier's decision.
type Signature struct {
	Version        int    `json:"version"`
	Algorithm      string `json:"algorithm"` // "ed25519"
	KeyID          string `json:"key_id"`    // first 16 hex of sha256(PKIX public key)
	ManifestSHA256 string `json:"manifest_sha256"`
	Signature      string `json:"signature"` // base64 (std)
}

// KeyID returns the short fingerprint of a public key.
func KeyID(pub ed25519.PublicKey) string {
	der, err := x509.MarshalPKIXPublicKey(pub)
	if err != nil {
		return ""
	}
	sum := sha256.Sum256(der)
	return hex.EncodeToString(sum[:8])
}

// GenerateKeyPair creates a new Ed25519 key pair and writes
// <prefix>.key (PKCS#8 PEM, mode 0600) and <prefix>.pub (PKIX PEM). It
// refuses to overwrite existing files.
func GenerateKeyPair(prefix string) (privPath, pubPath string, err error) {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return "", "", err
	}
	privDER, err := x509.MarshalPKCS8PrivateKey(priv)
	if err != nil {
		return "", "", err
	}
	pubDER, err := x509.MarshalPKIXPublicKey(pub)
	if err != nil {
		return "", "", err
	}
	privPath, pubPath = prefix+".key", prefix+".pub"
	if err := writeNew(privPath, pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: privDER}), 0o600); err != nil {
		return "", "", err
	}
	if err := writeNew(pubPath, pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: pubDER}), 0o644); err != nil {
		_ = os.Remove(privPath)
		return "", "", err
	}
	return privPath, pubPath, nil
}

func writeNew(path string, data []byte, mode os.FileMode) error {
	// #nosec G304 -- operator-chosen output path; O_EXCL prevents overwrite.
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, mode)
	if err != nil {
		return err
	}
	if _, err := f.Write(data); err != nil {
		_ = f.Close()
		_ = os.Remove(path)
		return err
	}
	return f.Close()
}

// LoadPrivateKey reads a PKCS#8 PEM Ed25519 private key. Keys readable by
// group or others are refused (POSIX only), as ssh does.
func LoadPrivateKey(path string) (ed25519.PrivateKey, error) {
	f, info, err := openRegular(path)
	if err != nil {
		return nil, err
	}
	_ = f.Close()
	if runtime.GOOS != "windows" && info.Mode().Perm()&0o077 != 0 {
		return nil, fmt.Errorf("private key %q has mode %#o; it must not be accessible to group or others (chmod 600)", path, info.Mode().Perm())
	}
	data, err := readSmallFile(path, 16<<10)
	if err != nil {
		return nil, err
	}
	blk, _ := pem.Decode(data)
	if blk == nil || blk.Type != "PRIVATE KEY" {
		return nil, errors.New("private key: expected a PKCS#8 PEM block of type PRIVATE KEY")
	}
	k, err := x509.ParsePKCS8PrivateKey(blk.Bytes)
	if err != nil {
		return nil, fmt.Errorf("private key: %w", err)
	}
	priv, ok := k.(ed25519.PrivateKey)
	if !ok {
		return nil, errors.New("private key is not Ed25519")
	}
	return priv, nil
}

// LoadPublicKey reads a PKIX PEM Ed25519 public key.
func LoadPublicKey(path string) (ed25519.PublicKey, error) {
	data, err := readSmallFile(path, 16<<10)
	if err != nil {
		return nil, err
	}
	blk, _ := pem.Decode(data)
	if blk == nil || blk.Type != "PUBLIC KEY" {
		return nil, errors.New("public key: expected a PKIX PEM block of type PUBLIC KEY")
	}
	k, err := x509.ParsePKIXPublicKey(blk.Bytes)
	if err != nil {
		return nil, fmt.Errorf("public key: %w", err)
	}
	pub, ok := k.(ed25519.PublicKey)
	if !ok {
		return nil, errors.New("public key is not Ed25519")
	}
	return pub, nil
}

// SignManifest signs the exact manifest bytes.
func SignManifest(manifestBytes []byte, priv ed25519.PrivateKey) (*Signature, error) {
	if len(priv) != ed25519.PrivateKeySize {
		return nil, errors.New("invalid private key")
	}
	sum := sha256.Sum256(manifestBytes)
	sig := ed25519.Sign(priv, append([]byte(signaturePrefix), manifestBytes...))
	return &Signature{
		Version:        SignatureVersion,
		Algorithm:      "ed25519",
		KeyID:          KeyID(priv.Public().(ed25519.PublicKey)),
		ManifestSHA256: hex.EncodeToString(sum[:]),
		Signature:      base64.StdEncoding.EncodeToString(sig),
	}, nil
}

// VerifyManifestSignature checks sig against manifestBytes with pub.
func VerifyManifestSignature(manifestBytes []byte, sig *Signature, pub ed25519.PublicKey) error {
	if sig == nil {
		return errors.New("no signature")
	}
	if len(pub) != ed25519.PublicKeySize {
		return errors.New("invalid public key")
	}
	if sig.Version != SignatureVersion {
		return fmt.Errorf("unsupported signature version %d", sig.Version)
	}
	if sig.Algorithm != "ed25519" {
		return fmt.Errorf("unsupported signature algorithm %q", sig.Algorithm)
	}
	raw, err := base64.StdEncoding.DecodeString(sig.Signature)
	if err != nil || len(raw) != ed25519.SignatureSize {
		return errors.New("malformed signature value")
	}
	sum := sha256.Sum256(manifestBytes)
	if sig.ManifestSHA256 != hex.EncodeToString(sum[:]) {
		return errors.New("manifest was modified after signing (digest differs)")
	}
	if !ed25519.Verify(pub, append([]byte(signaturePrefix), manifestBytes...), raw) {
		return fmt.Errorf("signature does not verify with public key %s", KeyID(pub))
	}
	return nil
}

// LoadSignature reads a detached signature file (JSON, at most 16 KiB,
// unknown fields rejected).
func LoadSignature(path string) (*Signature, error) {
	data, err := readSmallFile(path, 16<<10)
	if err != nil {
		return nil, fmt.Errorf("signature: %w", err)
	}
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	var s Signature
	if err := dec.Decode(&s); err != nil {
		return nil, fmt.Errorf("signature: %w", err)
	}
	return &s, nil
}

// WriteSignature writes sig as JSON to path (mode 0644, never overwrites).
func WriteSignature(path string, sig *Signature) error {
	data, err := json.MarshalIndent(sig, "", "  ")
	if err != nil {
		return err
	}
	return writeNew(path, append(data, '\n'), 0o644)
}
