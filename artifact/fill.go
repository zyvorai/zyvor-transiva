// Copyright 2026 Zyvor AI Labs · https://zyvor.dev
// SPDX-License-Identifier: LicenseRef-Zyvor-Production-2.0

package artifact

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/zyvorai/transiva/manifest"
)

// FillOptions controls FillChecksums.
type FillOptions struct {
	// OutPath receives the updated manifest. Empty rewrites the input in place
	// (atomically, keeping its permissions). A non-empty OutPath is never
	// overwritten.
	OutPath string
	// Root confines disk paths; empty means unconfined (CLI use).
	Root   string
	Limits Limits
}

// FillResult is the outcome of FillChecksums.
type FillResult struct {
	OutPath  string    `json:"out_path"`
	Filled   []string  `json:"filled"`  // disk IDs that gained a checksum
	Written  bool      `json:"written"` // false when nothing needed filling
	Findings []Finding `json:"findings"`
	SigStale bool      `json:"signature_stale"` // a <manifest>.sig exists and no longer matches
}

// FillChecksums computes SHA-256 for disks whose manifest entry has none and
// writes the manifest back. It never replaces an existing checksum: a
// recorded value is left alone (use VerifyBundle to check it). A file that
// cannot be read is reported and its disk left without a checksum. Filling
// trusts the file as it is now, so it records "what is here", not "what was
// exported"; the findings say so. Rewriting changes the manifest bytes, which
// invalidates any detached signature.
func FillChecksums(ctx context.Context, manifestPath string, opt FillOptions) (*FillResult, error) {
	lim := opt.Limits.withDefaults()
	data, err := readSmallFile(manifestPath, lim.MaxManifestBytes)
	if err != nil {
		return nil, fmt.Errorf("read manifest: %w", err)
	}
	yamlFmt := false
	var m *manifest.ArtifactManifest
	if l := strings.ToLower(manifestPath); strings.HasSuffix(l, ".yaml") || strings.HasSuffix(l, ".yml") {
		yamlFmt = true
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

	res := &FillResult{Filled: []string{}, Findings: []Finding{}}
	add := func(sev, code, subj, format string, args ...any) {
		res.Findings = append(res.Findings, Finding{Severity: sev, Code: code, Subject: subj, Message: fmt.Sprintf(format, args...)})
	}
	baseDir := filepath.Dir(manifestPath)
	for i := range m.Disks {
		d := &m.Disks[i]
		if d.Checksum != "" {
			continue
		}
		path := d.LocalPath
		if !filepath.IsAbs(path) {
			path = filepath.Join(baseDir, path)
		}
		if opt.Root != "" {
			p, err := ConfinePath(opt.Root, path)
			if err != nil {
				add(SeverityError, "path_outside_root", d.ID, "local_path %q rejected: %v", d.LocalPath, err)
				continue
			}
			path = p
		}
		f, info, err := openRegular(path)
		if err != nil {
			add(SeverityError, "file_unreadable", d.ID, "%v", err)
			continue
		}
		if info.Size() > lim.MaxDiskBytes {
			_ = f.Close()
			add(SeverityError, "file_too_large", d.ID, "file is %d bytes, over the %d byte cap", info.Size(), lim.MaxDiskBytes)
			continue
		}
		h := sha256.New()
		_, cerr := io.Copy(h, ctxReader{ctx, io.LimitReader(f, info.Size())})
		_ = f.Close()
		if cerr != nil {
			return res, cerr
		}
		d.Checksum = "sha256:" + hex.EncodeToString(h.Sum(nil))
		res.Filled = append(res.Filled, d.ID)
	}
	if len(res.Filled) == 0 {
		add(SeverityInfo, "nothing_to_fill", "", "every readable disk already has a checksum or could not be read; manifest not rewritten")
		return res, nil
	}
	add(SeverityWarning, "records_current_bytes", "", "checksums describe the files as they are now, not necessarily as exported")

	var out []byte
	if yamlFmt {
		out, err = manifest.ToYAML(m)
	} else {
		out, err = manifest.ToJSON(m)
	}
	if err != nil {
		return res, fmt.Errorf("serialize manifest: %w", err)
	}
	if opt.OutPath != "" {
		res.OutPath = opt.OutPath
		if err := writeExclusive(opt.OutPath, out); err != nil {
			return res, err
		}
	} else {
		res.OutPath = manifestPath
		if err := replaceFile(manifestPath, out); err != nil {
			return res, err
		}
		if _, err := os.Stat(manifestPath + ".sig"); err == nil {
			res.SigStale = true
			add(SeverityWarning, "signature_stale", "", "%s.sig no longer matches the rewritten manifest; sign it again", manifestPath)
		}
	}
	res.Written = true
	return res, nil
}

func writeExclusive(path string, data []byte) error {
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644) // #nosec G302,G304 -- operator-chosen output path
	if err != nil {
		return err
	}
	if _, err := f.Write(data); err != nil {
		_ = f.Close()
		return err
	}
	return f.Close()
}

// replaceFile atomically replaces an existing regular file, keeping its mode.
func replaceFile(path string, data []byte) error {
	info, err := os.Stat(path)
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() {
		return errors.New("manifest is not a regular file")
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".manifest-*.tmp")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	cleanup := func() { _ = os.Remove(tmpName) }
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		cleanup()
		return err
	}
	if err := tmp.Chmod(info.Mode().Perm()); err != nil {
		_ = tmp.Close()
		cleanup()
		return err
	}
	if err := tmp.Close(); err != nil {
		cleanup()
		return err
	}
	if err := os.Rename(tmpName, path); err != nil {
		cleanup()
		return err
	}
	return nil
}
