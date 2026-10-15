// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import "testing"

func TestBackupCoverageReasons(t *testing.T) {
	f := newFixture(t)
	out, stderr, err := runCLI(t, "backup", "coverage", "--include-disks", "--json")
	if err != nil {
		t.Fatalf("backup coverage: %v\n%s", err, stderr)
	}
	rep := decodeJSON[coverageReport](t, out)
	got := map[string]coverageRow{}
	for _, r := range rep.Rows {
		got[r.Reason+":"+r.Name+":"+r.Disk] = r
	}
	if r, ok := got["disabled-job-only:mail:"]; !ok || !r.Disagrees || r.UpstreamReported || len(r.DisabledJobs) != 1 || r.DisabledJobs[0] != "job2" {
		t.Errorf("102 must be disabled-job-only and flagged as an upstream disagreement: %+v", r)
	}
	if r, ok := got["excluded:dev:"]; !ok || r.Disagrees || !r.UpstreamReported {
		t.Errorf("103 must be excluded (upstream agrees): %+v", r)
	}
	if _, ok := got["disk-excluded:files:scsi1"]; !ok {
		t.Errorf("104 scsi1 (backup=no) must be listed with --include-disks; rows=%v", got)
	}
	if len(rep.Rows) != 3 || rep.Uncovered != 2 {
		t.Errorf("rows=%d uncovered=%d, want 3 and 2: %+v", len(rep.Rows), rep.Uncovered, rep.Rows)
	}
	// Negative: covered guests never appear, templates are skipped by default.
	for _, r := range rep.Rows {
		if r.VMID == 100 || r.VMID == 105 || r.VMID == 106 {
			t.Errorf("covered guest or template %d reported: %+v", r.VMID, r)
		}
	}
	out, _, err = runCLI(t, "backup", "coverage", "--include-templates", "--protected-elsewhere-tag", "veeam", "--json")
	if err != nil {
		t.Fatal(err)
	}
	rep = decodeJSON[coverageReport](t, out)
	reasons := map[int64]string{}
	for _, r := range rep.Rows {
		reasons[r.VMID] = r.Reason
	}
	if reasons[106] != "template" || reasons[103] != "" || reasons[102] != "disabled-job-only" {
		t.Errorf("template/tag handling: %v", reasons)
	}
	if f.writeCalls.Load() != 0 {
		t.Fatal("non-GET request issued")
	}
}
