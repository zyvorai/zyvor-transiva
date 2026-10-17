// Copyright 2026 Zyvor AI Labs · https://zyvor.dev
// SPDX-License-Identifier: Apache-2.0

package api

import (
	"testing"

	"github.com/zyvorai/transiva/daemon/models"
)

func TestJobProviderAndVMNameForTheDashboard(t *testing.T) {
	vsphere := &models.Job{Definition: models.JobDefinition{Name: "n", VMPath: "/DC0/vm/web"}}
	if jobProvider(vsphere) != "vsphere" || jobVMName(vsphere) != "/DC0/vm/web" {
		t.Errorf("legacy job: got %q / %q", jobProvider(vsphere), jobVMName(vsphere))
	}
	nutanix := &models.Job{Definition: models.JobDefinition{Provider: "nutanix", Name: "chimera-vm-01", VMPath: "00000000-0000-4000-8001-000000000001"}}
	if jobProvider(nutanix) != "nutanix" {
		t.Errorf("provider = %q, want nutanix", jobProvider(nutanix))
	}
	if jobVMName(nutanix) != "chimera-vm-01" {
		t.Errorf("vm name = %q, want the job name, not the UUID", jobVMName(nutanix))
	}
	unnamed := &models.Job{Definition: models.JobDefinition{Provider: "nutanix", VMPath: "uuid-only"}}
	if jobVMName(unnamed) != "uuid-only" {
		t.Errorf("unnamed job falls back to VMPath, got %q", jobVMName(unnamed))
	}
}
