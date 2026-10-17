// Copyright 2026 Zyvor AI Labs · https://zyvor.dev
// SPDX-License-Identifier: Apache-2.0

package nutanix

import (
	"regexp"
	"strconv"
	"strings"
)

// With the pipeline on, a job is export (0 to 40 percent) then h2kvm's stages. The percentages below place each
// h2kvm stage in the rest of the bar so the Jobs page keeps moving for the minutes the conversion takes.
const (
	exportShare      = 40.0
	stageInspect     = 40.0
	stageFix         = 45.0
	stageConvertFrom = 60.0
	stageConvertTo   = 85.0
	stageValidate    = 88.0
	stageLand        = 92.0
)

var (
	h2kvmStageRe    = regexp.MustCompile(`Stage: ([A-Z_ ]+?)\s*$`)
	h2kvmProgressRe = regexp.MustCompile(`(?:Conversion progress|Progress): ([0-9.]+)%`)
)

// pipelineProgress turns h2kvm's log lines into job progress.
type pipelineProgress struct {
	report func(phase string, percent float64, message string)
	stage  string
}

// scaleExport maps the export's own 0-100 percent onto its share of a pipeline job.
func scaleExport(percent float64, pipeline bool) float64 {
	if !pipeline {
		return percent
	}
	return percent * exportShare / 100
}

// Line handles one line of h2kvm output.
func (pp *pipelineProgress) Line(line string) {
	if m := h2kvmStageRe.FindStringSubmatch(line); m != nil {
		pp.stage = strings.TrimSpace(m[1])
		switch pp.stage {
		case "INSPECT":
			pp.report("inspecting", stageInspect, "h2kvm: inspecting the guest")
		case "FIX":
			pp.report("fixing", stageFix, "h2kvm: fixing boot, drivers and network")
		case "CONVERT":
			pp.report("converting", stageConvertFrom, "h2kvm: converting the disk")
		case "VALIDATE":
			pp.report("validating", stageValidate, "h2kvm: checking the converted disk")
		case "LIBVIRT INTEGRATION":
			pp.report("landing", stageLand, "h2kvm: defining and starting the VM on libvirt")
		}
		return
	}
	if pp.stage != "CONVERT" {
		return
	}
	if m := h2kvmProgressRe.FindStringSubmatch(line); m != nil {
		if pct, err := strconv.ParseFloat(m[1], 64); err == nil && pct >= 0 && pct <= 100 {
			overall := stageConvertFrom + pct*(stageConvertTo-stageConvertFrom)/100
			pp.report("converting", overall, "h2kvm: converting the disk ("+strconv.Itoa(int(pct))+"%)")
		}
	}
}
