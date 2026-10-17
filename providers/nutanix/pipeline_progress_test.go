// Copyright 2026 Zyvor AI Labs · https://zyvor.dev
// SPDX-License-Identifier: Apache-2.0

package nutanix

import "testing"

type progressEvent struct {
	phase   string
	percent float64
	message string
}

func replay(lines ...string) []progressEvent {
	var got []progressEvent
	pp := &pipelineProgress{report: func(phase string, percent float64, message string) {
		got = append(got, progressEvent{phase, percent, message})
	}}
	for _, l := range lines {
		pp.Line(l)
	}
	return got
}

// Lines as h2kvm 1.8 prints them in manifest mode (timestamp, level and process prefix included).
const (
	lineInspect  = "16:08:12 ✅ INFO     [proc=MainProcess] ➡️  Stage: INSPECT"
	lineFix      = "16:08:20 ✅ INFO     [proc=MainProcess] ➡️  Stage: FIX"
	lineConvert  = "16:09:14 ✅ INFO     [proc=MainProcess] ➡️  Stage: CONVERT"
	lineProg30   = "16:10:12 ✅ INFO     [proc=MainProcess] ⏳ Progress: 30.2%"
	lineConvProg = "16:10:14 ✅ INFO     [proc=MainProcess] ⏳ Conversion progress: 50.0%"
	lineValidate = "16:25:03 ✅ INFO     [proc=MainProcess] ➡️  Stage: VALIDATE"
	lineLibvirt  = "16:25:14 ✅ INFO     [proc=MainProcess] ➡️  Stage: LIBVIRT INTEGRATION"
)

func TestPipelineProgressFollowsTheStages(t *testing.T) {
	got := replay(lineInspect, lineFix, lineConvert, lineValidate, lineLibvirt)
	want := []progressEvent{
		{"inspecting", 40, "h2kvm: inspecting the guest"},
		{"fixing", 45, "h2kvm: fixing boot, drivers and network"},
		{"converting", 60, "h2kvm: converting the disk"},
		{"validating", 88, "h2kvm: checking the converted disk"},
		{"landing", 92, "h2kvm: defining and starting the VM on libvirt"},
	}
	if len(got) != len(want) {
		t.Fatalf("got %d events %v, want %d", len(got), got, len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("event %d = %+v, want %+v", i, got[i], want[i])
		}
	}
}

func TestConversionPercentIsScaledIntoTheBar(t *testing.T) {
	got := replay(lineConvert, lineProg30, lineConvProg)
	if len(got) != 3 {
		t.Fatalf("got %v", got)
	}
	// 60 + 30.2% of the 25-point convert span, then 50% of it.
	if got[1].percent < 67.4 || got[1].percent > 67.6 || got[2].percent != 72.5 {
		t.Errorf("percents = %v, %v; want about 67.55 and 72.5", got[1].percent, got[2].percent)
	}
	if got[2].phase != "converting" {
		t.Errorf("phase = %q", got[2].phase)
	}
}

func TestProgressLinesOutsideTheConvertStageAreIgnored(t *testing.T) {
	if got := replay(lineFix, lineProg30); len(got) != 1 {
		t.Errorf("a Progress line during FIX must not move the bar, got %v", got)
	}
	if got := replay("random log line", ""); len(got) != 0 {
		t.Errorf("unrelated lines must be ignored, got %v", got)
	}
}

func TestExportOccupiesItsShareOfAPipelineJob(t *testing.T) {
	if got := scaleExport(100, true); got != 40 {
		t.Errorf("scaleExport(100, pipeline) = %v, want 40", got)
	}
	if got := scaleExport(50, false); got != 50 {
		t.Errorf("a plain export keeps its own percent, got %v", got)
	}
}
