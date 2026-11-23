// Copyright 2026 Zyvor AI Labs · https://zyvor.dev
// SPDX-License-Identifier: LicenseRef-Zyvor-Production-2.0

package artifact

import (
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"path/filepath"
	"strings"

	"github.com/zyvorai/transiva/manifest"
)

// BundleOptions controls VerifyBundle.
type BundleOptions struct {
	// Root, when set, confines every disk path (after symlink resolution) to
	// this directory. The daemon always sets it; the CLI may leave it empty.
	Root string
	// SkipChecksums checks structure, sizes and headers only.
	SkipChecksums bool
	// RequireChecksums turns a disk without a checksum into an error.
	RequireChecksums bool
	// AllowBackingFiles downgrades "disk references a backing file" from error
	// to warning.
	AllowBackingFiles bool
	// PublicKey and SignatureFile, when both set, require a valid detached
	// signature over the manifest bytes.
	PublicKey     ed25519.PublicKey
	SignatureFile string
	Limits        Limits
}

// DiskReport is the per-disk result of VerifyBundle.
type DiskReport struct {
	ID               string     `json:"id"`
	Path             string     `json:"path"`
	DeclaredFormat   string     `json:"declared_format"`
	DeclaredBytes    int64      `json:"declared_bytes"`
	FileBytes        int64      `json:"file_bytes"`
	Sniff            *DiskSniff `json:"sniff,omitempty"`
	ExpectedChecksum string     `json:"expected_checksum,omitempty"`
	ActualChecksum   string     `json:"actual_checksum,omitempty"`
	ChecksumChecked  bool       `json:"checksum_checked"`
	ChecksumMatch    bool       `json:"checksum_match"`
}

// BundleReport is the result of VerifyBundle. OK is true only when there are
// no error-severity findings.
type BundleReport struct {
	ManifestPath      string       `json:"manifest_path"`
	ManifestSHA256    string       `json:"manifest_sha256,omitempty"`
	OK                bool         `json:"ok"`
	SignatureVerified bool         `json:"signature_verified"`
	SignatureKeyID    string       `json:"signature_key_id,omitempty"`
	Disks             []DiskReport `json:"disks"`
	Findings          []Finding    `json:"findings"`
	Disclaimer        string       `json:"disclaimer"`
}

// Disclaimer is attached to every report so consumers cannot mistake an
// integrity check for a boot or content guarantee.
const Disclaimer = "Offline integrity check only: files were read, never executed, mounted or booted. " +
	"A pass means the bytes match what the manifest recorded and headers agree with declared formats; " +
	"it does not prove the guest boots, is free of malware, or that the recorded checksums were taken from the original source."

// VerifyBundle loads the manifest at manifestPath and verifies the disks it
// lists. A manifest that cannot be read or parsed returns an error; every
// problem with the disks themselves is a Finding, and all disks are examined
// even after the first failure.
func VerifyBundle(ctx context.Context, manifestPath string, opt BundleOptions) (*BundleReport, error) {
	lim := opt.Limits.withDefaults()
	rep := &BundleReport{ManifestPath: manifestPath, Disclaimer: Disclaimer, Disks: []DiskReport{}, Findings: []Finding{}}

	data, err := readSmallFile(manifestPath, lim.MaxManifestBytes)
	if err != nil {
		return nil, fmt.Errorf("read manifest: %w", err)
	}
	sum := sha256.Sum256(data)
	rep.ManifestSHA256 = hex.EncodeToString(sum[:])

	var m *manifest.ArtifactManifest
	lower := strings.ToLower(manifestPath)
	if strings.HasSuffix(lower, ".yaml") || strings.HasSuffix(lower, ".yml") {
		m, err = manifest.FromYAML(data)
	} else {
		m, err = manifest.FromJSON(data)
	}
	if err != nil {
		return nil, fmt.Errorf("parse manifest: %w", err)
	}
	if err := manifest.ValidateSchema(m); err != nil {
		return nil, fmt.Errorf("invalid manifest: %w", err)
	}

	add := func(sev, code, subj, format string, args ...any) {
		rep.Findings = append(rep.Findings, Finding{Severity: sev, Code: code, Subject: subj, Message: fmt.Sprintf(format, args...)})
	}

	if opt.SignatureFile != "" || opt.PublicKey != nil {
		if opt.SignatureFile == "" || opt.PublicKey == nil {
			add(SeverityError, "signature_incomplete", "", "both a signature file and a public key are required to check a signature")
		} else if sig, err := LoadSignature(opt.SignatureFile); err != nil {
			add(SeverityError, "signature_unreadable", "", "%v", err)
		} else if err := VerifyManifestSignature(data, sig, opt.PublicKey); err != nil {
			add(SeverityError, "signature_invalid", "", "%v", err)
		} else {
			rep.SignatureVerified = true
			rep.SignatureKeyID = sig.KeyID
		}
	}

	baseDir := filepath.Dir(manifestPath)
	seenPaths := map[string]string{}

	for _, d := range m.Disks {
		if err := ctx.Err(); err != nil {
			return rep, err
		}
		dr := DiskReport{ID: d.ID, DeclaredFormat: d.SourceFormat, DeclaredBytes: d.Bytes, ExpectedChecksum: d.Checksum}

		path := d.LocalPath
		if !filepath.IsAbs(path) {
			path = filepath.Join(baseDir, path)
		}
		if opt.Root != "" {
			p, err := ConfinePath(opt.Root, path)
			if err != nil {
				add(SeverityError, "path_outside_root", d.ID, "local_path %q rejected: %v", d.LocalPath, err)
				rep.Disks = append(rep.Disks, dr)
				continue
			}
			path = p
		}
		dr.Path = path

		if prev, dup := seenPaths[path]; dup {
			add(SeverityError, "duplicate_path", d.ID, "same file as disk %q", prev)
		}
		seenPaths[path] = d.ID

		f, info, err := openRegular(path)
		if err != nil {
			add(SeverityError, "file_unreadable", d.ID, "%v", err)
			rep.Disks = append(rep.Disks, dr)
			continue
		}
		dr.FileBytes = info.Size()

		if info.Size() > lim.MaxDiskBytes {
			add(SeverityError, "file_too_large", d.ID, "file is %d bytes, over the %d byte cap", info.Size(), lim.MaxDiskBytes)
			_ = f.Close()
			rep.Disks = append(rep.Disks, dr)
			continue
		}

		sn, err := SniffDisk(f, info.Size())
		if err != nil {
			add(SeverityError, "header_unreadable", d.ID, "%v", err)
		} else {
			dr.Sniff = &sn
			checkSniff(add, d, sn, info.Size(), opt)
		}

		if d.Checksum == "" {
			sev := SeverityWarning
			if opt.RequireChecksums {
				sev = SeverityError
			}
			add(sev, "no_checksum", d.ID, "manifest records no checksum for this disk; integrity cannot be verified")
		} else if !opt.SkipChecksums {
			h := sha256.New()
			_, cerr := io.Copy(h, ctxReader{ctx, io.LimitReader(f, info.Size())})
			if cerr != nil {
				_ = f.Close()
				return rep, cerr
			}
			dr.ChecksumChecked = true
			dr.ActualChecksum = "sha256:" + hex.EncodeToString(h.Sum(nil))
			dr.ChecksumMatch = dr.ActualChecksum == d.Checksum
			if !dr.ChecksumMatch {
				add(SeverityError, "checksum_mismatch", d.ID, "expected %s, computed %s", d.Checksum, dr.ActualChecksum)
			}
		}
		_ = f.Close()
		rep.Disks = append(rep.Disks, dr)
	}

	rep.OK = !hasErrors(rep.Findings)
	return rep, nil
}

func checkSniff(add func(sev, code, subj, format string, args ...any), d manifest.DiskArtifact, sn DiskSniff, fileBytes int64, opt BundleOptions) {
	if sn.Format != d.SourceFormat {
		// a raw declaration over a file with no magic is the only agreement
		// for raw; any other disagreement is an error.
		add(SeverityError, "format_mismatch", d.ID, "manifest declares %q but the file header says %q", d.SourceFormat, sn.Format)
	}
	if sn.Encrypted {
		add(SeverityWarning, "encrypted", d.ID, "image header marks the disk as encrypted; downstream conversion needs the key")
	}
	if sn.BackingFile != "" {
		sev := SeverityError
		if opt.AllowBackingFiles {
			sev = SeverityWarning
		}
		add(sev, "backing_file", d.ID, "image references a backing file %q that is not covered by this manifest; a converter would read it from wherever it resolves", sn.BackingFile)
	}
	for _, e := range sn.ExtentFiles {
		if err := SafeEntryName(e); err != nil {
			add(SeverityError, "extent_path", d.ID, "vmdk extent %q is not a plain file name: %v", e, err)
		} else {
			add(SeverityWarning, "extent_not_covered", d.ID, "vmdk descriptor references extent file %q, which this manifest does not list or checksum", e)
		}
	}
	switch {
	case d.SourceFormat == "raw" && d.Bytes > 0 && fileBytes != d.Bytes:
		add(SeverityWarning, "size_mismatch", d.ID, "raw disk file is %d bytes but manifest declares %d", fileBytes, d.Bytes)
	case d.SourceFormat != "raw" && d.Bytes > 0 && fileBytes != d.Bytes && sn.VirtualBytes > 0 && sn.VirtualBytes != d.Bytes:
		add(SeverityWarning, "size_mismatch", d.ID, "manifest declares %d bytes; file is %d bytes and header virtual size is %d", d.Bytes, fileBytes, sn.VirtualBytes)
	}
}
