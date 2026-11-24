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
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/zyvorai/transiva/manifest"
)

// BuildOptions controls BuildManifest.
type BuildOptions struct {
	// Provider, VMName and ExportMethod fill manifest.source (optional).
	Provider     string
	VMName       string
	ExportMethod string
	// SkipChecksums leaves disk checksums empty instead of hashing.
	SkipChecksums bool
	Limits        Limits
}

var diskExts = map[string]bool{
	".vmdk": true, ".qcow2": true, ".img": true, ".raw": true,
	".vhd": true, ".vhdx": true, ".vdi": true,
}

var idCleanRe = regexp.MustCompile(`[^a-zA-Z0-9_-]+`)

// BuildManifest creates an Artifact Manifest for disk files found directly in
// dir (not recursive), for exports made by other tools such as ovftool or
// govc. Formats come from file headers, never from extensions alone; sizes
// are the header's virtual size when known, else the file size; checksums are
// SHA-256 of the file. Paths are recorded relative to dir so the manifest can
// be written there and moved with the bundle. Files that cannot be used are
// reported as findings rather than silently skipped. Nothing is executed.
func BuildManifest(ctx context.Context, dir string, opt BuildOptions) (*manifest.ArtifactManifest, []Finding, error) {
	lim := opt.Limits.withDefaults()
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, nil, fmt.Errorf("read directory: %w", err)
	}
	var findings []Finding
	add := func(sev, code, subj, f string, a ...any) {
		findings = append(findings, Finding{Severity: sev, Code: code, Subject: subj, Message: fmt.Sprintf(f, a...)})
	}

	var names []string
	for _, e := range entries {
		if e.IsDir() || !diskExts[strings.ToLower(filepath.Ext(e.Name()))] {
			continue
		}
		names = append(names, e.Name())
	}
	sort.Strings(names)

	m := &manifest.ArtifactManifest{ManifestVersion: "1.0"}
	if opt.Provider != "" {
		now := time.Now().UTC()
		m.Source = &manifest.SourceMetadata{Provider: opt.Provider, VMName: opt.VMName, ExportMethod: opt.ExportMethod, ExportTimestamp: &now}
	}
	used := map[string]bool{}
	for _, name := range names {
		if err := ctx.Err(); err != nil {
			return nil, findings, err
		}
		full := filepath.Join(dir, name)
		f, info, err := openRegular(full) // refuses symlinks to devices, FIFOs
		if err != nil {
			add(SeverityWarning, "file_skipped", name, "%v", err)
			continue
		}
		if info.Size() > lim.MaxDiskBytes {
			_ = f.Close()
			add(SeverityWarning, "file_skipped", name, "file is %d bytes, over the %d byte cap", info.Size(), lim.MaxDiskBytes)
			continue
		}
		sn, err := SniffDisk(f, info.Size())
		if err != nil {
			_ = f.Close()
			add(SeverityWarning, "file_skipped", name, "header unreadable: %v", err)
			continue
		}
		ext := strings.ToLower(filepath.Ext(name))
		if sn.Format == "raw" && ext != ".img" && ext != ".raw" {
			_ = f.Close()
			add(SeverityWarning, "file_skipped", name, "extension %s but no matching header (likely a flat extent or truncated file); not listed", ext)
			continue
		}
		if sn.BackingFile != "" {
			add(SeverityWarning, "backing_file", name, "image references backing file %q that is not part of this manifest", sn.BackingFile)
		}
		for _, e := range sn.ExtentFiles {
			add(SeverityWarning, "extent_not_covered", name, "vmdk descriptor references extent %q which is not listed", e)
		}
		bytesN := info.Size()
		if sn.VirtualBytes > 0 {
			bytesN = sn.VirtualBytes
		}
		d := manifest.DiskArtifact{SourceFormat: sn.Format, Bytes: bytesN, LocalPath: name}
		if !opt.SkipChecksums {
			h := sha256.New()
			if _, err := io.Copy(h, ctxReader{ctx, io.LimitReader(f, info.Size())}); err != nil {
				_ = f.Close()
				return nil, findings, err
			}
			d.Checksum = "sha256:" + hex.EncodeToString(h.Sum(nil))
		}
		_ = f.Close()

		id := idCleanRe.ReplaceAllString(strings.TrimSuffix(name, filepath.Ext(name)), "-")
		id = strings.Trim(id, "-_")
		if id == "" {
			id = "disk"
		}
		base, n := id, 1
		for used[id] {
			n++
			id = fmt.Sprintf("%s-%d", base, n)
		}
		used[id] = true
		d.ID = id
		d.BootOrderHint = len(m.Disks)
		m.Disks = append(m.Disks, d)
	}
	if len(m.Disks) == 0 {
		return nil, findings, errors.New("no usable disk files found (looked for .vmdk .qcow2 .img .raw .vhd .vhdx .vdi directly in the directory)")
	}
	add(SeverityInfo, "boot_order", "", "boot_order_hint follows file-name order and is a guess; edit the manifest if the boot disk is not first")
	if err := manifest.ValidateSchema(m); err != nil {
		return nil, findings, fmt.Errorf("generated manifest is invalid: %w", err)
	}
	return m, findings, nil
}

// Change is one difference between two manifests.
type Change struct {
	Kind    string `json:"kind"`    // added, removed, changed
	Subject string `json:"subject"` // disk id, or "source"
	Field   string `json:"field,omitempty"`
	Old     string `json:"old,omitempty"`
	New     string `json:"new,omitempty"`
}

// ManifestDiff is the result of DiffManifests.
type ManifestDiff struct {
	Identical bool     `json:"identical"`
	Changes   []Change `json:"changes"`
}

// DiffManifests compares two manifests disk by disk (matched by ID). Local
// paths are compared only when includePaths is true, since a bundle moved to
// another host legitimately changes them. Use it to see whether a re-export
// or a copy differs from the original in format, size or checksum.
func DiffManifests(a, b *manifest.ArtifactManifest, includePaths bool) ManifestDiff {
	var ch []Change
	chg := func(subj, field, o, n string) {
		if o != n {
			ch = append(ch, Change{Kind: "changed", Subject: subj, Field: field, Old: o, New: n})
		}
	}
	chg("manifest", "manifest_version", a.ManifestVersion, b.ManifestVersion)
	var sa, sb manifest.SourceMetadata
	if a.Source != nil {
		sa = *a.Source
	}
	if b.Source != nil {
		sb = *b.Source
	}
	chg("source", "provider", sa.Provider, sb.Provider)
	chg("source", "vm_id", sa.VMID, sb.VMID)
	chg("source", "vm_name", sa.VMName, sb.VMName)

	am := map[string]manifest.DiskArtifact{}
	bm := map[string]manifest.DiskArtifact{}
	var ids []string
	for _, d := range a.Disks {
		am[d.ID] = d
		ids = append(ids, d.ID)
	}
	for _, d := range b.Disks {
		if _, ok := am[d.ID]; !ok {
			ids = append(ids, d.ID)
		}
		bm[d.ID] = d
	}
	sort.Strings(ids)
	for _, id := range ids {
		da, inA := am[id]
		db, inB := bm[id]
		switch {
		case !inB:
			ch = append(ch, Change{Kind: "removed", Subject: id})
		case !inA:
			ch = append(ch, Change{Kind: "added", Subject: id})
		default:
			chg(id, "source_format", da.SourceFormat, db.SourceFormat)
			chg(id, "bytes", fmt.Sprint(da.Bytes), fmt.Sprint(db.Bytes))
			chg(id, "checksum", da.Checksum, db.Checksum)
			chg(id, "boot_order_hint", fmt.Sprint(da.BootOrderHint), fmt.Sprint(db.BootOrderHint))
			chg(id, "disk_type", da.DiskType, db.DiskType)
			if includePaths {
				chg(id, "local_path", da.LocalPath, db.LocalPath)
			}
		}
	}
	if ch == nil {
		ch = []Change{}
	}
	return ManifestDiff{Identical: len(ch) == 0, Changes: ch}
}

// WriteSHA256Sums writes the manifest's recorded disk checksums in the
// coreutils `sha256sum -c` format, so any tool can re-verify a bundle without
// Transiva. Paths are written relative to the manifest directory when the
// disk lives under it. Disks without a checksum are returned in skipped; a
// path that cannot be represented safely (newline or backslash) is an error.
func WriteSHA256Sums(w io.Writer, m *manifest.ArtifactManifest, manifestDir string) (skipped []string, err error) {
	for _, d := range m.Disks {
		if d.Checksum == "" {
			skipped = append(skipped, d.ID)
			continue
		}
		hexsum := strings.TrimPrefix(d.Checksum, "sha256:")
		if len(hexsum) != 64 {
			return skipped, fmt.Errorf("disk %q: malformed checksum", d.ID)
		}
		p := d.LocalPath
		if filepath.IsAbs(p) {
			if rel, rerr := filepath.Rel(manifestDir, p); rerr == nil && !strings.HasPrefix(rel, "..") {
				p = rel
			}
		}
		if strings.ContainsAny(p, "\n\r\\") {
			return skipped, fmt.Errorf("disk %q: path contains a newline or backslash and cannot be written safely", d.ID)
		}
		if _, err := fmt.Fprintf(w, "%s  %s\n", strings.ToLower(hexsum), filepath.ToSlash(p)); err != nil {
			return skipped, err
		}
	}
	return skipped, nil
}
