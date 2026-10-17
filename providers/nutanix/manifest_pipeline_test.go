// Copyright 2026 Zyvor AI Labs · https://zyvor.dev
// SPDX-License-Identifier: Apache-2.0

package nutanix

import (
	"os"
	"path/filepath"
	"testing"
)

func pipelineTestDisk(t *testing.T) []ConvertedDisk {
	t.Helper()
	p := filepath.Join(t.TempDir(), "disk.qcow2")
	if err := os.WriteFile(p, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	return []ConvertedDisk{{OutputPath: p, Bytes: 1}}
}

func TestManifestHasNoPipelineSectionByDefault(t *testing.T) {
	m, err := BuildArtifactManifest(PickupVM{Name: "vm", UUID: "u", VCPUs: 2, MemoryGiB: 4}, pipelineTestDisk(t), "qcow2")
	if err != nil {
		t.Fatal(err)
	}
	if m.Pipeline != nil || m.Output != nil {
		t.Errorf("a plain export must not enable h2kvm stages: pipeline=%v output=%v", m.Pipeline, m.Output)
	}
}

func TestManifestEnablesAllH2KVMStagesWhenAPipelineIsRequested(t *testing.T) {
	conv := filepath.Join(t.TempDir(), "converted")
	m, err := BuildArtifactManifestWithPipeline(PickupVM{Name: "vm", UUID: "u", VCPUs: 2, MemoryGiB: 4}, pipelineTestDisk(t), "qcow2", conv)
	if err != nil {
		t.Fatal(err)
	}
	p := m.Pipeline
	if p == nil || !p.Inspect.Enabled || !p.Fix.Enabled || !p.Convert.Enabled || !p.Validate.Enabled {
		t.Fatalf("all four stages must be enabled, got %+v", p)
	}
	if m.Output == nil || m.Output.Directory != conv || m.Output.Format != "qcow2" {
		t.Errorf("output = %+v, want directory %q and format qcow2", m.Output, conv)
	}
}
