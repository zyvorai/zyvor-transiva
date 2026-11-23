// Copyright 2026 Zyvor AI Labs · https://zyvor.dev
// SPDX-License-Identifier: LicenseRef-Zyvor-Production-2.0

package artifact

import (
	"bytes"
	"encoding/binary"
	"io"
	"regexp"
	"strings"
)

// DiskSniff is what the header of a disk file says about itself. It is read
// from at most the first 64 KiB and the last 512 bytes; the file is never
// parsed beyond that and never opened by qemu or any other tool.
type DiskSniff struct {
	Format       string   `json:"format"`                  // vmdk, qcow2, vhdx, vhd, vdi, raw (no known magic)
	Variant      string   `json:"variant,omitempty"`       // e.g. "sparse", "descriptor"
	VirtualBytes int64    `json:"virtual_bytes,omitempty"` // 0 when the header does not say
	BackingFile  string   `json:"backing_file,omitempty"`  // qcow2 backing file / vmdk parent hint
	ExtentFiles  []string `json:"extent_files,omitempty"`  // vmdk descriptor extents
	Encrypted    bool     `json:"encrypted,omitempty"`
}

const sniffHead = 64 << 10

var (
	vmdkExtentRe = regexp.MustCompile(`^(?:RW|RDONLY|NOACCESS)\s+\d+\s+\w+\s+"([^"]*)"`)
	vmdkParentRe = regexp.MustCompile(`^parentFileNameHint\s*=\s*"([^"]*)"`)
	vmdkCreateRe = regexp.MustCompile(`^createType\s*=\s*"([^"]*)"`)
)

// SniffDisk inspects the header of r (size bytes long).
func SniffDisk(r io.ReaderAt, size int64) (DiskSniff, error) {
	n := int64(sniffHead)
	if size < n {
		n = size
	}
	head := make([]byte, n)
	if n > 0 {
		if _, err := r.ReadAt(head, 0); err != nil && err != io.EOF {
			return DiskSniff{}, err
		}
	}

	switch {
	case len(head) >= 4 && bytes.Equal(head[:4], []byte{'Q', 'F', 'I', 0xfb}):
		return sniffQcow2(head), nil
	case len(head) >= 4 && bytes.Equal(head[:4], []byte("KDMV")):
		s := DiskSniff{Format: "vmdk", Variant: "sparse"}
		if len(head) >= 20 {
			// capacity in 512-byte sectors at offset 12
			s.VirtualBytes = clampMul(binary.LittleEndian.Uint64(head[12:20]), 512)
		}
		return s, nil
	case len(head) >= 8 && bytes.Equal(head[:8], []byte("vhdxfile")):
		return DiskSniff{Format: "vhdx"}, nil
	case len(head) >= 0x44 && binary.LittleEndian.Uint32(head[0x40:0x44]) == 0xbeda107f:
		s := DiskSniff{Format: "vdi"}
		if len(head) >= 0x178 {
			// disk size in bytes in the main header
			s.VirtualBytes = clampMul(binary.LittleEndian.Uint64(head[0x170:0x178]), 1)
		}
		return s, nil
	case bytes.HasPrefix(head, []byte("# Disk DescriptorFile")):
		return sniffVMDKDescriptor(head), nil
	}

	// VHD: footer ("conectix") in the last 512 bytes; dynamic disks also
	// carry a copy at offset 0.
	if bytes.HasPrefix(head, []byte("conectix")) {
		return DiskSniff{Format: "vhd"}, nil
	}
	if size >= 512 {
		tail := make([]byte, 512)
		if _, err := r.ReadAt(tail, size-512); err == nil || err == io.EOF {
			if bytes.HasPrefix(tail, []byte("conectix")) {
				s := DiskSniff{Format: "vhd"}
				// current size, big-endian, at footer offset 48
				s.VirtualBytes = clampMul(binary.BigEndian.Uint64(tail[48:56]), 1)
				return s, nil
			}
		}
	}
	return DiskSniff{Format: "raw"}, nil
}

func sniffQcow2(h []byte) DiskSniff {
	s := DiskSniff{Format: "qcow2"}
	if len(h) < 36 {
		return s
	}
	backOff := binary.BigEndian.Uint64(h[8:16])
	backLen := binary.BigEndian.Uint32(h[16:20])
	s.VirtualBytes = clampMul(binary.BigEndian.Uint64(h[24:32]), 1)
	s.Encrypted = binary.BigEndian.Uint32(h[32:36]) != 0
	if backOff != 0 && backLen != 0 {
		if backLen > 1023 {
			backLen = 1023 // qcow2 spec maximum
		}
		end := backOff + uint64(backLen)
		if backOff < uint64(len(h)) && end <= uint64(len(h)) {
			s.BackingFile = printable(string(h[backOff:end]))
		} else {
			s.BackingFile = "(backing file name outside the header window)"
		}
	}
	return s
}

func sniffVMDKDescriptor(h []byte) DiskSniff {
	s := DiskSniff{Format: "vmdk", Variant: "descriptor"}
	// descriptors are small text files; stop at the first NUL
	if i := bytes.IndexByte(h, 0); i >= 0 {
		h = h[:i]
	}
	for _, line := range strings.Split(string(h), "\n") {
		line = strings.TrimSpace(line)
		if m := vmdkParentRe.FindStringSubmatch(line); m != nil && m[1] != "" && m[1] != "none" {
			s.BackingFile = printable(m[1])
		}
		if m := vmdkExtentRe.FindStringSubmatch(line); m != nil {
			s.ExtentFiles = append(s.ExtentFiles, printable(m[1]))
		}
		if m := vmdkCreateRe.FindStringSubmatch(line); m != nil {
			s.Variant = "descriptor/" + printable(m[1])
		}
	}
	return s
}

// clampMul multiplies and clamps to int64 range so hostile headers cannot
// produce negative sizes.
func clampMul(v uint64, m uint64) int64 {
	const max = uint64(1<<63 - 1)
	if m != 0 && v > max/m {
		return int64(max)
	}
	return int64(v * m)
}

// printable strips control characters from header-derived strings before they
// reach logs or JSON.
func printable(s string) string {
	return strings.Map(func(r rune) rune {
		if r < 0x20 || r == 0x7f {
			return -1
		}
		return r
	}, s)
}
