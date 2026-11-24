// Copyright 2026 Zyvor AI Labs · https://zyvor.dev
// SPDX-License-Identifier: LicenseRef-Zyvor-Production-2.0

package artifact

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/zyvorai/transiva/manifest"
)

// Readiness values, modelled on the Ready / Conditionally ready / Not ready
// verdicts that Azure Migrate and Forklift show before a migration runs.
const (
	ReadinessReady       = "ready"
	ReadinessConditional = "conditionally_ready"
	ReadinessNotReady    = "not_ready"
)

// AssessReport is the result of AssessManifest. It is derived from manifest
// metadata only: no disk is opened and no hypervisor is contacted.
type AssessReport struct {
	Readiness  string    `json:"readiness"`
	Errors     int       `json:"errors"`
	Warnings   int       `json:"warnings"`
	Infos      int       `json:"infos"`
	Findings   []Finding `json:"findings"`
	Disclaimer string    `json:"disclaimer"`
}

// AssessDisclaimer states the limits of a metadata-only assessment.
const AssessDisclaimer = "Metadata-only pre-flight assessment of the Artifact Manifest: no disk was read and no guest was inspected. " +
	"Findings are hints about what the receiving side (for example h2kvm) will have to handle; they do not predict that a guest will boot."

var (
	macRe     = regexp.MustCompile(`^[0-9A-Fa-f]{2}([:-][0-9A-Fa-f]{2}){5}$`)
	dns1123Re = regexp.MustCompile(`^[a-z0-9]([-a-z0-9]*[a-z0-9])?(\.[a-z0-9]([-a-z0-9]*[a-z0-9])?)*$`)
)

const (
	assessHugeDisk = int64(16) << 40 // 16 TiB
	assessBigDisk  = int64(2) << 40  // 2 TiB (MBR addressing limit)
)

// AssessManifest applies readiness rules to a validated manifest, in the style
// of Forklift validation concerns (Critical / Warning / Information, here
// error / warning / info). Not ready means at least one error; conditionally
// ready means warnings only.
func AssessManifest(m *manifest.ArtifactManifest) *AssessReport {
	rep := &AssessReport{Findings: []Finding{}, Disclaimer: AssessDisclaimer}
	add := func(sev, code, subj, format string, args ...any) {
		rep.Findings = append(rep.Findings, Finding{Severity: sev, Code: code, Subject: subj, Message: fmt.Sprintf(format, args...)})
	}

	if m.VM == nil {
		add(SeverityWarning, "no_vm_metadata", "", "manifest has no vm section: firmware, CPU, memory and OS are unknown; the receiver will assume BIOS")
	} else {
		fw := strings.ToLower(m.VM.Firmware)
		switch fw {
		case "uefi", "efi":
			add(SeverityInfo, "uefi_firmware", "", "guest boots with UEFI; the target needs OVMF firmware")
			if m.VM.SecureBoot {
				add(SeverityWarning, "secure_boot", "", "Secure Boot is enabled; the target needs OVMF with Secure Boot variables and signed bootloaders")
			}
		case "", "unknown":
			add(SeverityWarning, "firmware_unknown", "", "firmware type is not recorded; BIOS will be assumed and a UEFI guest will not boot")
		}
		switch strings.ToLower(m.VM.OSHint) {
		case "windows":
			add(SeverityWarning, "windows_guest", "", "Windows guest: VirtIO storage and network drivers must be injected before first boot on KVM")
		case "", "unknown":
			add(SeverityInfo, "os_unknown", "", "guest OS is not recorded, so OS-specific fixes cannot be chosen automatically")
		}
		if m.VM.CPU <= 0 || m.VM.MemGB <= 0 {
			add(SeverityInfo, "no_sizing", "", "vCPU count or memory is not recorded; the target VM must be sized by hand")
		}
	}

	if m.Source != nil && m.Source.VMName != "" && len(m.Source.VMName) <= 253 && !dns1123Re.MatchString(m.Source.VMName) {
		add(SeverityInfo, "vm_name_not_dns1123", "", "VM name %q is not a valid DNS-1123 name; targets such as KubeVirt will need it renamed", m.Source.VMName)
	}

	if len(m.Disks) > 1 {
		add(SeverityInfo, "multiple_disks", "", "%d disks: confirm the boot disk and attachment order", len(m.Disks))
		boot := 0
		for _, d := range m.Disks {
			if d.DiskType == "boot" {
				boot++
			}
		}
		if boot == 0 {
			add(SeverityWarning, "no_boot_disk", "", "no disk is marked disk_type boot; boot order is ambiguous")
		} else if boot > 1 {
			add(SeverityWarning, "multiple_boot_disks", "", "%d disks are marked disk_type boot", boot)
		}
	}
	for _, d := range m.Disks {
		switch d.SourceFormat {
		case "qcow2", "raw":
		default:
			add(SeverityInfo, "needs_conversion", d.ID, "%s disk must be converted for KVM (qcow2 or raw)", d.SourceFormat)
		}
		switch {
		case d.Bytes > assessHugeDisk:
			add(SeverityError, "disk_too_large", d.ID, "disk is %d bytes (over 16 TiB), beyond what common targets accept", d.Bytes)
		case d.Bytes > assessBigDisk:
			add(SeverityWarning, "disk_over_2tib", d.ID, "disk is over 2 TiB: a BIOS/MBR guest cannot address it fully and copies will be slow")
		}
		if d.Checksum == "" {
			add(SeverityWarning, "no_checksum", d.ID, "no checksum recorded; integrity of the copy cannot be verified")
		}
	}

	macs := map[string]string{}
	for i, n := range m.NICs {
		subj := n.ID
		if subj == "" {
			subj = fmt.Sprintf("nic-%d", i)
		}
		if n.MAC == "" {
			add(SeverityInfo, "nic_no_mac", subj, "no MAC address recorded; the guest may get a new one and lose MAC-bound network config")
			continue
		}
		if !macRe.MatchString(n.MAC) {
			add(SeverityWarning, "nic_bad_mac", subj, "MAC %q is not a valid address", n.MAC)
			continue
		}
		key := strings.ToLower(strings.ReplaceAll(n.MAC, "-", ":"))
		if prev, dup := macs[key]; dup {
			add(SeverityError, "nic_duplicate_mac", subj, "MAC %s is also used by %s", n.MAC, prev)
		}
		macs[key] = subj
	}
	if len(m.NICs) == 0 {
		add(SeverityInfo, "no_nics", "", "no network interfaces recorded")
	}

	for _, w := range m.Warnings {
		add(SeverityWarning, "export_warning", w.Stage, "reported during export: %s", w.Message)
	}

	rep.Readiness = ReadinessReady
	for _, f := range rep.Findings {
		switch f.Severity {
		case SeverityError:
			rep.Errors++
		case SeverityWarning:
			rep.Warnings++
		default:
			rep.Infos++
		}
	}
	if rep.Errors > 0 {
		rep.Readiness = ReadinessNotReady
	} else if rep.Warnings > 0 {
		rep.Readiness = ReadinessConditional
	}
	return rep
}

// LoadManifest reads, parses (JSON, or YAML by extension) and schema-validates
// a manifest file without touching the disks it lists.
func LoadManifest(path string, lim Limits) (*manifest.ArtifactManifest, error) {
	lim = lim.withDefaults()
	data, err := readSmallFile(path, lim.MaxManifestBytes)
	if err != nil {
		return nil, fmt.Errorf("read manifest: %w", err)
	}
	var m *manifest.ArtifactManifest
	if l := strings.ToLower(path); strings.HasSuffix(l, ".yaml") || strings.HasSuffix(l, ".yml") {
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
	return m, nil
}
