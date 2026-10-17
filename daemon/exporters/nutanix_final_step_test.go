// Copyright 2026 Zyvor AI Labs · https://zyvor.dev
// SPDX-License-Identifier: Apache-2.0

package exporters

import (
	"strings"
	"testing"

	"github.com/zyvorai/transiva/providers"
)

func TestFinalNutanixStep(t *testing.T) {
	cases := []struct {
		name      string
		meta      map[string]interface{}
		requested map[string]interface{}
		wantStep  string
		wantErr   string
	}{
		{"plain export", nil, nil, "Export completed", ""},
		{"pipeline converted", map[string]interface{}{"pipeline_success": true}, nil, "Migration complete: converted by h2kvm", ""},
		{"pipeline converted and landed", map[string]interface{}{"pipeline_success": true}, map[string]interface{}{"land_on_libvirt": true},
			"Migration complete: converted by h2kvm and started on this host", ""},
		{"pipeline failed", map[string]interface{}{"pipeline_error": "h2kvm failed: exit status 1", "pipeline_success": false}, nil,
			"", "exported, but the h2kvm pipeline failed: h2kvm failed: exit status 1"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			step, err := finalNutanixStep(&providers.ExportResult{Metadata: c.meta}, c.requested)
			if c.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), c.wantErr) {
					t.Fatalf("err = %v, want %q", err, c.wantErr)
				}
				return
			}
			if err != nil || step != c.wantStep {
				t.Errorf("step, err = %q, %v; want %q", step, err, c.wantStep)
			}
		})
	}
}
