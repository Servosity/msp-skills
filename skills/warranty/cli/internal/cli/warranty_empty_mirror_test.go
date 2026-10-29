// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-written test (handfixes.json warranty-novels).

package cli

import (
	"strings"
	"testing"
)

// An empty mirror, or a filter that matches no device, is never an all-clear:
// every read command says why it has nothing to show.
func TestWarrantyEmptyMirrorIsNotAllClear(t *testing.T) {
	if inFreshProcess(t) {
		return
	}
	warrantyTestEnv(t, defaultFakeVendor())

	type noted struct {
		Checked int    `json:"checked"`
		Note    string `json:"note"`
	}
	for _, args := range [][]string{{"unresolved", "--json"}, {"report", "--json"}} {
		out, errOut, err := execWarranty(t, "", args...)
		if err != nil {
			t.Fatalf("%v on a fresh mirror: %v\n%s", args, err, errOut)
		}
		var v noted
		decodeJSON(t, out, &v)
		if !strings.Contains(v.Note, "mirror is empty") {
			t.Errorf("%v on a fresh mirror: note = %q", args, v.Note)
		}
	}
	if _, errOut, err := execWarranty(t, "", "lifecycle", "--json"); err != nil || !strings.Contains(errOut, "hint: the warranty mirror has no devices") {
		t.Errorf("lifecycle on a fresh mirror: err=%v stderr=%q", err, errOut)
	}

	if _, errOut, err := execWarranty(t, testImportCSV, "import", "-", "--customer", "Acme Dental", "--json"); err != nil {
		t.Fatalf("import: %v\n%s", err, errOut)
	}
	out, errOut, err := execWarranty(t, "", "unresolved", "--customer", "Nobody Inc", "--json")
	if err != nil {
		t.Fatalf("unresolved for an unknown customer: %v\n%s", err, errOut)
	}
	var v noted
	decodeJSON(t, out, &v)
	if v.Checked != 0 || !strings.Contains(v.Note, "no devices in the mirror for this filter") {
		t.Errorf("unresolved for an unknown customer = %+v", v)
	}

	// recheck names the device it leaves out (no known vendor) instead of dropping it silently.
	_, errOut, err = execWarranty(t, "", "recheck", "--dry-run")
	if err != nil {
		t.Fatalf("recheck --dry-run: %v\n%s", err, errOut)
	}
	if !strings.Contains(errOut, "hint: 1 device has no known vendor") {
		t.Errorf("recheck --dry-run stderr = %q", errOut)
	}
}
