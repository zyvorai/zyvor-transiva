// Copyright 2026 Zyvor AI Labs · https://zyvor.dev
// SPDX-License-Identifier: LicenseRef-Zyvor-Production-2.0

package artifact

import (
	"fmt"
	"math"
	"os"
	"path/filepath"

	"github.com/zyvorai/transiva/manifest"
)

// Fit verdicts of EstimateSpace.
const (
	FitUnknown = "unknown" // free space not supplied
	FitYes     = "fits"
	FitMaybe   = "may_not_fit"
	FitNo      = "does_not_fit"
)

// EstimateOptions controls EstimateSpace.
type EstimateOptions struct {
	// TargetFormat is "qcow2" (default) or "raw".
	TargetFormat string
	// FreeBytes is the space available at the destination; negative means unknown.
	FreeBytes int64
	// Root confines disk paths (the daemon sets it); empty means unconfined.
	Root string
	// ManifestDir resolves relative local_path values.
	ManifestDir string
}

// DiskEstimate is the per-disk part of a SpaceEstimate. Sizes are bytes.
type DiskEstimate struct {
	ID             string `json:"id"`
	SourceFormat   string `json:"source_format"`
	VirtualBytes   int64  `json:"virtual_bytes"`
	FileBytes      int64  `json:"file_bytes"` // -1 when the file is missing
	MinTargetBytes int64  `json:"min_target_bytes"`
	MaxTargetBytes int64  `json:"max_target_bytes"`
	Note           string `json:"note,omitempty"`
}

// SpaceEstimate is the result of EstimateSpace.
type SpaceEstimate struct {
	TargetFormat  string         `json:"target_format"`
	Disks         []DiskEstimate `json:"disks"`
	MinTotalBytes int64          `json:"min_total_bytes"`
	MaxTotalBytes int64          `json:"max_total_bytes"`
	FreeBytes     int64          `json:"free_bytes"` // -1 when unknown
	Fit           string         `json:"fit"`
	Findings      []Finding      `json:"findings"`
	Disclaimer    string         `json:"disclaimer"`
}

// EstimateDisclaimer explains how rough the numbers are.
const EstimateDisclaimer = "Rough bounds computed from declared virtual sizes and on-disk file sizes; no image was converted or measured. " +
	"The authoritative figure is `qemu-img measure` run by the receiving side."

// qcow2Overhead is the allowance (in 1/N of virtual size) added to a fully
// allocated qcow2 image for L1/L2 tables and refcounts: about 1.6%.
const qcow2OverheadDiv = 64

// EstimateSpace bounds the space a converted copy of every disk will need.
// For a raw target the need is the full virtual size. For qcow2 the lower
// bound is the source file size (data already present, capped at the virtual
// size) and the upper bound is the virtual size plus metadata overhead. The
// manifest-declared size is trusted; files are only stat-ed, never opened.
func EstimateSpace(m *manifest.ArtifactManifest, opt EstimateOptions) (*SpaceEstimate, error) {
	tf := opt.TargetFormat
	if tf == "" {
		tf = "qcow2"
	}
	if tf != "qcow2" && tf != "raw" {
		return nil, fmt.Errorf("unsupported target format %q (use qcow2 or raw)", tf)
	}
	est := &SpaceEstimate{TargetFormat: tf, Disks: []DiskEstimate{}, FreeBytes: opt.FreeBytes, Findings: []Finding{}, Disclaimer: EstimateDisclaimer}
	if est.FreeBytes < 0 {
		est.FreeBytes = -1
	}
	add := func(sev, code, subj, format string, args ...any) {
		est.Findings = append(est.Findings, Finding{Severity: sev, Code: code, Subject: subj, Message: fmt.Sprintf(format, args...)})
	}
	for _, d := range m.Disks {
		de := DiskEstimate{ID: d.ID, SourceFormat: d.SourceFormat, VirtualBytes: d.Bytes, FileBytes: -1}
		path := d.LocalPath
		if !filepath.IsAbs(path) {
			path = filepath.Join(opt.ManifestDir, path)
		}
		if opt.Root != "" {
			p, err := ConfinePath(opt.Root, path)
			if err != nil {
				add(SeverityError, "path_outside_root", d.ID, "local_path %q rejected: %v", d.LocalPath, err)
				path = ""
			} else {
				path = p
			}
		}
		if path != "" {
			if info, err := os.Stat(path); err == nil && info.Mode().IsRegular() {
				de.FileBytes = info.Size()
			} else {
				add(SeverityWarning, "file_missing", d.ID, "disk file not found or not a regular file; using the declared size only")
			}
		}
		switch tf {
		case "raw":
			de.MinTargetBytes, de.MaxTargetBytes = d.Bytes, d.Bytes
			de.Note = "raw output is fully allocated unless the filesystem keeps it sparse"
		default:
			upper := satAdd(d.Bytes, d.Bytes/qcow2OverheadDiv)
			lower := d.Bytes
			if de.FileBytes >= 0 && de.FileBytes < d.Bytes {
				lower = de.FileBytes
			}
			de.MinTargetBytes, de.MaxTargetBytes = lower, upper
		}
		est.MinTotalBytes = satAdd(est.MinTotalBytes, de.MinTargetBytes)
		est.MaxTotalBytes = satAdd(est.MaxTotalBytes, de.MaxTargetBytes)
		est.Disks = append(est.Disks, de)
	}
	switch {
	case est.FreeBytes < 0:
		est.Fit = FitUnknown
	case est.MaxTotalBytes <= est.FreeBytes:
		est.Fit = FitYes
	case est.MinTotalBytes <= est.FreeBytes:
		est.Fit = FitMaybe
		add(SeverityWarning, "may_not_fit", "", "upper bound %d bytes exceeds the %d bytes available; it fits only if the guest data stays sparse", est.MaxTotalBytes, est.FreeBytes)
	default:
		est.Fit = FitNo
		add(SeverityError, "does_not_fit", "", "even the lower bound %d bytes exceeds the %d bytes available", est.MinTotalBytes, est.FreeBytes)
	}
	return est, nil
}

// satAdd adds non-negative sizes, saturating at MaxInt64 instead of wrapping.
func satAdd(a, b int64) int64 {
	if a < 0 || b < 0 {
		return math.MaxInt64
	}
	if a > math.MaxInt64-b {
		return math.MaxInt64
	}
	return a + b
}
