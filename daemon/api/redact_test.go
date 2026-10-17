// Copyright 2026 Zyvor AI Labs · https://zyvor.dev
// SPDX-License-Identifier: Apache-2.0

package api

import (
	"testing"

	"github.com/zyvorai/transiva/daemon/models"
)

func TestRedactedJobHidesCredentialsButKeepsTheRest(t *testing.T) {
	job := &models.Job{Definition: models.JobDefinition{
		Name: "chimera-vm-01",
		Metadata: map[string]interface{}{
			"server":   "prism.example.com",
			"username": "admin",
			"password": "hunter2",
			"mounts":   map[string]interface{}{"default-container": "/mnt/nutanix/default"},
			"nested":   map[string]interface{}{"api_key": "k", "keep": "v"},
		},
	}}

	got := redactedJob(job)

	if got.Definition.Metadata["password"] != redactedValue {
		t.Errorf("password = %v, want %q", got.Definition.Metadata["password"], redactedValue)
	}
	nested := got.Definition.Metadata["nested"].(map[string]interface{})
	if nested["api_key"] != redactedValue || nested["keep"] != "v" {
		t.Errorf("nested = %v", nested)
	}
	for _, k := range []string{"server", "username"} {
		if got.Definition.Metadata[k] != job.Definition.Metadata[k] {
			t.Errorf("%s should be unchanged", k)
		}
	}
	if job.Definition.Metadata["password"] != "hunter2" {
		t.Error("the stored job must not be modified; the daemon still needs the password to run it")
	}
}

func TestRedactedJobsAndNil(t *testing.T) {
	if redactedJob(nil) != nil {
		t.Error("nil stays nil")
	}
	jobs := redactedJobs([]*models.Job{{Definition: models.JobDefinition{Metadata: map[string]interface{}{"password": "x"}}}})
	if jobs[0].Definition.Metadata["password"] != redactedValue {
		t.Error("jobs in a list must be redacted too")
	}
}
