// Copyright 2026 Zyvor AI Labs · https://zyvor.dev
// SPDX-License-Identifier: LicenseRef-Zyvor-Production-2.0

package artifact

import (
	"archive/tar"
	"bytes"
	"context"
	"crypto/sha1" // #nosec G505 -- OVF manifests may use SHA1; only used to verify, never to sign
	"crypto/sha256"
	"crypto/sha512"
	"encoding/hex"
	"encoding/xml"
	"errors"
	"fmt"
	"hash"
	"io"
	"regexp"
	"strings"
)

// OVAEntry describes one archive member.
type OVAEntry struct {
	Name   string `json:"name"`
	Bytes  int64  `json:"bytes"`
	SHA256 string `json:"sha256,omitempty"`
}

// OVAReport is the result of InspectOVA.
type OVAReport struct {
	Path           string     `json:"path"`
	OK             bool       `json:"ok"`
	Entries        []OVAEntry `json:"entries"`
	VirtualSystem  string     `json:"virtual_system,omitempty"`
	DiskFiles      []string   `json:"disk_files,omitempty"`
	ManifestDigest string     `json:"manifest_algorithm,omitempty"`
	ManifestFound  bool       `json:"manifest_found"`
	ManifestOK     bool       `json:"manifest_verified"`
	CertPresent    bool       `json:"certificate_present"`
	Findings       []Finding  `json:"findings"`
	Disclaimer     string     `json:"disclaimer"`
}

// ovfEnvelope is the small slice of the OVF descriptor we read. Go's xml
// package does not expand entities or fetch external DTDs.
type ovfEnvelope struct {
	Files []struct {
		ID   string `xml:"id,attr"`
		Href string `xml:"href,attr"`
		Size string `xml:"size,attr"`
	} `xml:"References>File"`
	Disks []struct {
		DiskID  string `xml:"diskId,attr"`
		FileRef string `xml:"fileRef,attr"`
	} `xml:"DiskSection>Disk"`
	VirtualSystem struct {
		Name string `xml:"Name"`
	} `xml:"VirtualSystem"`
}

var mfLineRe = regexp.MustCompile(`^(SHA1|SHA256|SHA512)\((.+)\)\s*=\s*([0-9a-fA-F]+)\s*$`)

const maxMFBytes = 4 << 20

// InspectOVA streams the OVA at path (a plain, uncompressed tar) and checks
// it without extracting anything to disk: entry names and types, size and
// count caps, the OVF descriptor's file references, and every digest listed
// in the .mf manifest. A certificate (.cert) is reported but its signature is
// NOT verified.
func InspectOVA(ctx context.Context, path string, lim Limits) (*OVAReport, error) {
	lim = lim.withDefaults()
	f, info, err := openRegular(path)
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()

	rep := &OVAReport{Path: path, Entries: []OVAEntry{}, Findings: []Finding{}, Disclaimer: Disclaimer}
	add := func(sev, code, subj, format string, args ...any) {
		rep.Findings = append(rep.Findings, Finding{Severity: sev, Code: code, Subject: subj, Message: fmt.Sprintf(format, args...)})
	}

	magic := make([]byte, 2)
	if n, _ := f.ReadAt(magic, 0); n == 2 && magic[0] == 0x1f && magic[1] == 0x8b {
		add(SeverityError, "compressed", "", "archive is gzip-compressed; OVA files must be plain tar. Not decompressed: decompress deliberately with a size limit if you trust the source")
		return rep, nil
	}
	if info.Size() > lim.MaxTotalBytes {
		add(SeverityError, "archive_too_large", "", "archive is %d bytes, over the %d byte cap", info.Size(), lim.MaxTotalBytes)
		return rep, nil
	}

	computed := map[string]map[string]string{}
	sizes := map[string]int64{}
	var ovfData, mfData []byte
	var ovfName, mfName string
	seen := map[string]bool{}
	var total int64

	tr := tar.NewReader(ctxReader{ctx, f})
	for idx := 0; ; idx++ {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			add(SeverityError, "tar_corrupt", "", "archive is truncated or not a tar file: %v", err)
			break
		}
		if err := ctx.Err(); err != nil {
			return rep, err
		}
		name := hdr.Name
		if idx >= lim.MaxEntries {
			add(SeverityError, "too_many_entries", "", "more than %d entries; stopped reading", lim.MaxEntries)
			break
		}
		if hdr.Typeflag == tar.TypeDir {
			add(SeverityWarning, "directory_entry", name, "directory entry ignored; OVA members are flat files")
			continue
		}
		if hdr.Typeflag != tar.TypeReg {
			add(SeverityError, "entry_type", printable(name), "entry type %q (link, device or special file) is not allowed in an OVA", string(hdr.Typeflag))
			continue
		}
		if err := SafeEntryName(name); err != nil {
			add(SeverityError, "entry_name", printable(name), "unsafe entry name: %v", err)
			continue
		}
		key := strings.ToLower(name)
		if seen[key] {
			add(SeverityError, "duplicate_entry", name, "entry appears more than once (case-insensitive); a later entry could shadow the one that was checked")
			continue
		}
		seen[key] = true
		if hdr.Size < 0 || hdr.Size > lim.MaxDiskBytes {
			add(SeverityError, "entry_too_large", name, "entry declares %d bytes, over the %d byte cap; not read", hdr.Size, lim.MaxDiskBytes)
			continue
		}
		total += hdr.Size
		if total > lim.MaxTotalBytes {
			add(SeverityError, "archive_too_large", "", "entries total over %d bytes; stopped reading", lim.MaxTotalBytes)
			break
		}

		ext := strings.ToLower(name[strings.LastIndex(name, ".")+1:])
		switch {
		case ext == "ovf" || ext == "mf":
			if hdr.Size > lim.MaxManifestBytes || (ext == "mf" && hdr.Size > maxMFBytes) {
				add(SeverityError, "descriptor_too_large", name, "%d bytes is over the descriptor cap", hdr.Size)
				continue
			}
			b, err := io.ReadAll(io.LimitReader(tr, hdr.Size))
			if err != nil {
				add(SeverityError, "tar_corrupt", name, "read: %v", err)
				continue
			}
			sizes[name] = hdr.Size
			if ext == "ovf" {
				if ovfName != "" {
					add(SeverityError, "multiple_ovf", name, "second OVF descriptor; only one is allowed")
					continue
				}
				ovfName, ovfData = name, b
				if idx != 0 {
					add(SeverityWarning, "ovf_not_first", name, "OVF descriptor is entry %d; the OVF specification requires it first", idx)
				}
			} else {
				if mfName != "" {
					add(SeverityError, "multiple_manifest", name, "second .mf manifest")
					continue
				}
				mfName, mfData = name, b
			}
			rep.Entries = append(rep.Entries, OVAEntry{Name: name, Bytes: hdr.Size})
		default:
			if ext == "cert" {
				rep.CertPresent = true
			}
			h1, h256, h512 := sha1.New(), sha256.New(), sha512.New() // #nosec G401
			n, err := io.Copy(io.MultiWriter(h1, h256, h512), io.LimitReader(tr, hdr.Size))
			if err != nil {
				if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
					return rep, err
				}
				add(SeverityError, "tar_corrupt", name, "read: %v", err)
				continue
			}
			if n != hdr.Size {
				add(SeverityError, "tar_corrupt", name, "entry shorter than its header (%d of %d bytes)", n, hdr.Size)
				continue
			}
			sizes[name] = n
			computed[name] = map[string]string{
				"SHA1": hex.EncodeToString(h1.Sum(nil)), "SHA256": hex.EncodeToString(h256.Sum(nil)), "SHA512": hex.EncodeToString(h512.Sum(nil)),
			}
			rep.Entries = append(rep.Entries, OVAEntry{Name: name, Bytes: n, SHA256: computed[name]["SHA256"]})
		}
	}

	if ovfName == "" {
		add(SeverityError, "no_ovf", "", "archive contains no .ovf descriptor")
	} else {
		inspectOVF(rep, add, ovfData, sizes, lim)
	}

	if mfName == "" {
		add(SeverityWarning, "no_manifest", "", "archive has no .mf manifest; there are no recorded digests to verify")
	} else {
		rep.ManifestFound = true
		verifyMF(rep, add, mfName, mfData, computed, ovfName, ovfData)
	}
	if rep.CertPresent {
		add(SeverityInfo, "certificate_unverified", "", "a .cert signature file is present; Transiva does not validate X.509 signatures or trust chains")
	}

	rep.OK = !hasErrors(rep.Findings)
	return rep, nil
}

func inspectOVF(rep *OVAReport, add func(sev, code, subj, format string, args ...any), ovfData []byte, sizes map[string]int64, lim Limits) {
	var env ovfEnvelope
	dec := xml.NewDecoder(bytes.NewReader(ovfData))
	dec.Strict = true
	if err := dec.Decode(&env); err != nil {
		add(SeverityError, "ovf_parse", "", "OVF descriptor is not well-formed XML: %v", err)
		return
	}
	rep.VirtualSystem = printable(strings.TrimSpace(env.VirtualSystem.Name))
	refs := map[string]string{} // file id -> href
	for _, fr := range env.Files {
		if err := SafeEntryName(fr.Href); err != nil {
			add(SeverityError, "ovf_href", printable(fr.Href), "file reference is not a plain file name: %v", err)
			continue
		}
		refs[fr.ID] = fr.Href
		rep.DiskFiles = append(rep.DiskFiles, fr.Href)
		got, ok := sizes[fr.Href]
		if !ok {
			add(SeverityError, "ovf_file_missing", fr.Href, "referenced by the OVF but not present in the archive")
			continue
		}
		if fr.Size != "" {
			var want int64
			if _, err := fmt.Sscanf(fr.Size, "%d", &want); err != nil || want < 0 {
				add(SeverityError, "ovf_size", fr.Href, "unparseable size attribute %q", fr.Size)
			} else if want != got {
				add(SeverityError, "ovf_size_mismatch", fr.Href, "OVF says %d bytes, archive entry has %d", want, got)
			}
		}
	}
	for _, d := range env.Disks {
		if d.FileRef != "" {
			if _, ok := refs[d.FileRef]; !ok {
				add(SeverityError, "ovf_disk_ref", d.DiskID, "disk refers to file id %q which is not in References", d.FileRef)
			}
		}
	}
	referenced := map[string]bool{}
	for _, h := range refs {
		referenced[strings.ToLower(h)] = true
	}
	for name := range sizes {
		l := strings.ToLower(name)
		if strings.HasSuffix(l, ".ovf") || strings.HasSuffix(l, ".mf") || strings.HasSuffix(l, ".cert") {
			continue
		}
		if !referenced[l] {
			add(SeverityWarning, "unreferenced_entry", name, "archive member is not referenced by the OVF descriptor")
		}
	}
}

func verifyMF(rep *OVAReport, add func(sev, code, subj, format string, args ...any), mfName string, mfData []byte, computed map[string]map[string]string, ovfName string, ovfData []byte) {
	wantLen := map[string]int{"SHA1": 40, "SHA256": 64, "SHA512": 128}
	listed := map[string]bool{}
	problems := 0
	algs := map[string]bool{}
	for _, line := range strings.Split(strings.ReplaceAll(string(mfData), "\r\n", "\n"), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		m := mfLineRe.FindStringSubmatch(line)
		if m == nil {
			add(SeverityError, "mf_syntax", "", "unparseable manifest line %q", printable(truncate(line, 80)))
			problems++
			continue
		}
		alg, name, digest := m[1], m[2], strings.ToLower(m[3])
		algs[alg] = true
		if len(digest) != wantLen[alg] {
			add(SeverityError, "mf_digest_length", name, "%s digest has %d hex characters, want %d", alg, len(digest), wantLen[alg])
			problems++
			continue
		}
		if err := SafeEntryName(name); err != nil {
			add(SeverityError, "mf_name", printable(name), "manifest names an unsafe path: %v", err)
			problems++
			continue
		}
		var got string
		if name == ovfName {
			got = hashBytes(alg, ovfData)
		} else if c, ok := computed[name]; ok {
			got = c[alg]
		} else {
			add(SeverityError, "mf_file_missing", name, "listed in the manifest but not present in the archive")
			problems++
			continue
		}
		listed[name] = true
		if got != digest {
			add(SeverityError, "digest_mismatch", name, "%s digest differs from the manifest (manifest %s, computed %s)", alg, digest, got)
			problems++
		}
	}
	for name := range computed {
		l := strings.ToLower(name)
		if strings.HasSuffix(l, ".cert") {
			continue
		}
		if !listed[name] {
			add(SeverityWarning, "mf_not_listed", name, "archive member has no digest in the manifest and was not verified")
		}
	}
	var names []string
	for a := range algs {
		names = append(names, a)
	}
	rep.ManifestDigest = strings.Join(names, ",")
	rep.ManifestOK = problems == 0 && len(listed) > 0
	_ = mfName
}

func hashBytes(alg string, b []byte) string {
	var h hash.Hash
	switch alg {
	case "SHA1":
		h = sha1.New() // #nosec G401
	case "SHA512":
		h = sha512.New()
	default:
		h = sha256.New()
	}
	h.Write(b)
	return hex.EncodeToString(h.Sum(nil))
}

func truncate(s string, n int) string {
	if len(s) > n {
		return s[:n] + "..."
	}
	return s
}
