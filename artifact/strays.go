// Copyright 2026 Zyvor AI Labs · https://zyvor.dev
// SPDX-License-Identifier: LicenseRef-Zyvor-Production-2.0

package artifact

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

const maxStrayScan = 10000

// bundleSidecars are files that legitimately sit next to a manifest.
var bundleSidecars = map[string]bool{
	"sha256sums": true, "sha256sums.txt": true,
}

var strayDiskExts = map[string]bool{
	".vmdk": true, ".qcow2": true, ".img": true, ".raw": true, ".vhd": true,
	".vhdx": true, ".vdi": true, ".ova": true, ".ovf": true, ".iso": true,
}

// findStrays walks dir (never following symlinks) and reports files that no
// manifest disk references. Disk-looking files are warnings (they may be
// data that was exported but forgotten, or leftovers that inflate the
// bundle); anything else is informational. referenced holds resolved paths of
// listed disks; skip holds the manifest and its detached signature.
func findStrays(dir string, referenced map[string]string, skip map[string]bool, add func(sev, code, subj, format string, args ...any)) {
	// resolve symlinks in dir once so comparisons are like for like
	if real, err := filepath.EvalSymlinks(dir); err == nil {
		dir = real
	}
	seen := 0
	_ = filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			add(SeverityInfo, "stray_scan_error", "", "cannot scan %q: %v", path, err)
			if d != nil && d.IsDir() {
				return fs.SkipDir
			}
			return nil
		}
		if d.IsDir() {
			return nil
		}
		seen++
		if seen > maxStrayScan {
			add(SeverityInfo, "stray_scan_truncated", "", "stopped after %d entries", maxStrayScan)
			return fs.SkipAll
		}
		if referenced[path] != "" || skip[path] {
			return nil
		}
		rel, rerr := filepath.Rel(dir, path)
		if rerr != nil {
			rel = path
		}
		lower := strings.ToLower(d.Name())
		if bundleSidecars[lower] {
			return nil
		}
		if d.Type()&os.ModeSymlink != 0 {
			add(SeverityInfo, "stray_symlink", rel, "symlink not referenced by the manifest")
			return nil
		}
		if strayDiskExts[filepath.Ext(lower)] {
			add(SeverityWarning, "stray_disk_file", rel, "disk-like file is not listed in the manifest")
		} else {
			add(SeverityInfo, "stray_file", rel, "file is not listed in the manifest")
		}
		return nil
	})
}
