// Copyright 2026 Zyvor AI Labs · https://zyvor.dev
// SPDX-License-Identifier: LicenseRef-Zyvor-Production-2.0

// Package artifact verifies exported VM artifacts offline.
//
// Everything here reads bytes and compares them with what a manifest or an
// OVA descriptor claims. Nothing is executed, mounted, booted or converted,
// and no guest is contacted. Inputs are treated as hostile: sizes are capped,
// archive entry names are validated, and non-regular files are refused.
//
// What a passing result means: the files on disk are byte-identical to what
// the manifest (or OVA manifest) recorded, and the on-disk headers agree with
// the declared formats. It does not mean the guest boots, is free of malware,
// or that the checksums were recorded from the original source disk.
package artifact

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// Limits caps resource use when reading untrusted artifacts.
type Limits struct {
	MaxManifestBytes int64 // manifest / signature / OVF descriptor files
	MaxDiskBytes     int64 // largest single disk or archive entry accepted
	MaxTotalBytes    int64 // sum of all entries in one OVA
	MaxEntries       int   // archive entries in one OVA
}

// DefaultLimits returns conservative defaults: 8 MiB manifests, 16 TiB per
// disk, 64 TiB per archive and 256 archive entries.
func DefaultLimits() Limits {
	return Limits{
		MaxManifestBytes: 8 << 20,
		MaxDiskBytes:     16 << 40,
		MaxTotalBytes:    64 << 40,
		MaxEntries:       256,
	}
}

func (l Limits) withDefaults() Limits {
	d := DefaultLimits()
	if l.MaxManifestBytes <= 0 {
		l.MaxManifestBytes = d.MaxManifestBytes
	}
	if l.MaxDiskBytes <= 0 {
		l.MaxDiskBytes = d.MaxDiskBytes
	}
	if l.MaxTotalBytes <= 0 {
		l.MaxTotalBytes = d.MaxTotalBytes
	}
	if l.MaxEntries <= 0 {
		l.MaxEntries = d.MaxEntries
	}
	return l
}

// Severity of a Finding.
const (
	SeverityError   = "error"
	SeverityWarning = "warning"
	SeverityInfo    = "info"
)

// Finding is one observation about an artifact.
type Finding struct {
	Severity string `json:"severity"`
	Code     string `json:"code"`
	Subject  string `json:"subject,omitempty"` // disk id or archive entry
	Message  string `json:"message"`
}

func hasErrors(fs []Finding) bool {
	for _, f := range fs {
		if f.Severity == SeverityError {
			return true
		}
	}
	return false
}

// SafeEntryName reports whether name is acceptable as a flat OVA entry name or
// OVF file reference: non-empty, at most 255 bytes, no path separators, no
// ".." / "." names, no NUL or control characters, no drive-letter prefix and
// no leading "-".
func SafeEntryName(name string) error {
	switch {
	case name == "":
		return errors.New("empty name")
	case len(name) > 255:
		return errors.New("name longer than 255 bytes")
	case name == "." || name == "..":
		return errors.New("name is a relative path component")
	case strings.HasPrefix(name, "-"):
		return errors.New("name starts with '-'")
	case len(name) >= 2 && name[1] == ':':
		return errors.New("name has a drive-letter prefix")
	}
	for _, r := range name {
		switch {
		case r == '/' || r == '\\':
			return errors.New("name contains a path separator")
		case r < 0x20 || r == 0x7f:
			return errors.New("name contains a control character")
		}
	}
	if strings.Contains(name, "..") {
		return errors.New("name contains '..'")
	}
	return nil
}

// ConfinePath resolves p (absolute, or relative to root) with symlinks
// evaluated and returns it only if it lies inside root. Both must exist.
func ConfinePath(root, p string) (string, error) {
	if root == "" {
		return "", errors.New("no root directory configured")
	}
	if strings.ContainsRune(p, 0) || p == "" {
		return "", errors.New("invalid path")
	}
	rootAbs, err := filepath.Abs(root)
	if err != nil {
		return "", err
	}
	rootReal, err := filepath.EvalSymlinks(rootAbs)
	if err != nil {
		return "", fmt.Errorf("root: %w", err)
	}
	if !filepath.IsAbs(p) {
		p = filepath.Join(rootReal, p)
	}
	real, err := filepath.EvalSymlinks(filepath.Clean(p))
	if err != nil {
		return "", fmt.Errorf("path not accessible: %w", err)
	}
	rel, err := filepath.Rel(rootReal, real)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", errors.New("path escapes the allowed root")
	}
	return real, nil
}

// openRegular opens a regular file, refusing FIFOs, devices and directories
// (a FIFO would block forever, a device never ends).
func openRegular(path string) (*os.File, os.FileInfo, error) {
	if strings.ContainsRune(path, 0) {
		return nil, nil, errors.New("path contains a NUL byte")
	}
	pre, err := os.Stat(path)
	if err != nil {
		return nil, nil, err
	}
	if !pre.Mode().IsRegular() {
		return nil, nil, fmt.Errorf("%q is not a regular file", path)
	}
	// #nosec G304 -- caller supplies or confines the path.
	f, err := os.Open(path)
	if err != nil {
		return nil, nil, err
	}
	info, err := f.Stat()
	if err != nil {
		_ = f.Close()
		return nil, nil, err
	}
	if !info.Mode().IsRegular() {
		_ = f.Close()
		return nil, nil, fmt.Errorf("%q is not a regular file", path)
	}
	return f, info, nil
}

// readSmallFile reads a regular file of at most max bytes.
func readSmallFile(path string, max int64) ([]byte, error) {
	f, info, err := openRegular(path)
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()
	if info.Size() > max {
		return nil, fmt.Errorf("%q is %d bytes, over the %d byte limit", path, info.Size(), max)
	}
	data, err := io.ReadAll(io.LimitReader(f, max+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > max {
		return nil, fmt.Errorf("%q is over the %d byte limit", path, max)
	}
	return data, nil
}

// ctxReader makes io.Copy stop promptly when ctx is cancelled.
type ctxReader struct {
	ctx context.Context
	r   io.Reader
}

func (c ctxReader) Read(p []byte) (int, error) {
	if err := c.ctx.Err(); err != nil {
		return 0, err
	}
	return c.r.Read(p)
}

// ReadSmallFile reads a regular file of at most max bytes; directories,
// devices, FIFOs and oversized files are refused.
func ReadSmallFile(path string, max int64) ([]byte, error) { return readSmallFile(path, max) }
