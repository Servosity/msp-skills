// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
//
// The schema guard.
//
// This test is the structural fix for the failure that produced these commands'
// first cut: attribute names were invented, and the fixtures used the same
// invented names, so the suite passed while every command was wrong against real
// data. Here every name the code reads is checked against the CLI's own shipped
// spec.json. A field Auvik does not emit cannot pass this test, so it cannot
// reach a user.

package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"testing"
)

type specDoc struct {
	Types map[string]struct {
		Fields []struct {
			Name string   `json:"name"`
			Type string   `json:"type"`
			Enum []string `json:"enum"`
		} `json:"fields"`
	} `json:"types"`
}

func loadSpecDoc(t *testing.T) specDoc {
	t.Helper()
	// spec.json is written to the CLI root at generate time. It is NOT vendored
	// into msp-skills (546KB of generator input, and no skill in the fleet ships
	// one), so the guard runs where the spec lives -- the printing-press working
	// copy -- and skips here rather than failing a checkout that was never meant
	// to carry it. If you are chasing an Auvik schema drift, run these tests in
	// the press working dir where spec.json sits beside the CLI root.
	path := filepath.Join("..", "..", "spec.json")
	raw, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		t.Skipf("skipping schema guard: %s is not vendored into this repo", path)
	}
	if err != nil {
		t.Fatalf("cannot read %s: %v (the schema guard requires the shipped spec)", path, err)
	}
	var doc specDoc
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatalf("cannot parse spec.json: %v", err)
	}
	if len(doc.Types) == 0 {
		t.Fatal("spec.json declares no types; the schema guard cannot verify anything")
	}
	return doc
}

// TestAuvikSpecFieldsExist fails when the code reads an attribute the API does
// not declare. This is the test that would have caught the invented-field bug.
func TestAuvikSpecFieldsExist(t *testing.T) {
	doc := loadSpecDoc(t)

	for _, f := range auvikSpecFields {
		typ, ok := doc.Types[f.Type]
		if !ok {
			t.Errorf("spec type %q does not exist (referenced for field %q)", f.Type, f.Field)
			continue
		}
		found := false
		names := make([]string, 0, len(typ.Fields))
		for _, sf := range typ.Fields {
			names = append(names, sf.Name)
			if sf.Name == f.Field {
				found = true
			}
		}
		if !found {
			sort.Strings(names)
			t.Errorf("%s.%s is NOT declared by the Auvik spec.\n    declared fields: %v",
				f.Type, f.Field, names)
		}
	}
}

// TestAuvikSpecRelsExist does the same for JSON:API relationships.
func TestAuvikSpecRelsExist(t *testing.T) {
	doc := loadSpecDoc(t)

	for _, r := range auvikSpecRels {
		typ, ok := doc.Types[r.Type]
		if !ok {
			t.Errorf("spec type %q does not exist (referenced for relationship %q)", r.Type, r.Name)
			continue
		}
		found := false
		names := make([]string, 0, len(typ.Fields))
		for _, sf := range typ.Fields {
			names = append(names, sf.Name)
			if sf.Name == r.Name {
				found = true
			}
		}
		if !found {
			sort.Strings(names)
			t.Errorf("%s.%s is NOT declared by the Auvik spec.\n    declared: %v",
				r.Type, r.Name, names)
		}
	}
}

// TestAuvikStatusEnumsMatchSpec pins the enum values the bucketing logic branches
// on. If Auvik adds or renames a state, this fails rather than silently
// mis-classifying a device.
func TestAuvikStatusEnumsMatchSpec(t *testing.T) {
	doc := loadSpecDoc(t)

	enumOf := func(typeName, fieldName string) []string {
		typ, ok := doc.Types[typeName]
		if !ok {
			t.Fatalf("spec type %q missing", typeName)
		}
		for _, f := range typ.Fields {
			if f.Name == fieldName {
				return f.Enum
			}
		}
		t.Fatalf("%s.%s missing", typeName, fieldName)
		return nil
	}

	lifecycle := enumOf(fLifecycleLastSupportStatus.Type, fLifecycleLastSupportStatus.Field)
	wantLifecycle := map[string]bool{
		"covered": true, "available": true, "expired": true,
		"securityOnly": true, "unpublished": true, "empty": true,
	}
	if len(lifecycle) != len(wantLifecycle) {
		t.Errorf("lifecycle status enum changed: %v (bucketing logic may be stale)", lifecycle)
	}
	for _, v := range lifecycle {
		if !wantLifecycle[v] {
			t.Errorf("unhandled lifecycle status %q — statusMeansExpired/SecurityOnly need review", v)
		}
	}
	// The two values the code actually branches on must still be present.
	if !containsStr(lifecycle, "expired") {
		t.Error("lifecycle enum no longer contains 'expired'; statusMeansExpired is dead")
	}
	if !containsStr(lifecycle, "securityOnly") {
		t.Error("lifecycle enum no longer contains 'securityOnly'")
	}

	probe := enumOf(fDiscoverySNMP.Type, fDiscoverySNMP.Field)
	for _, v := range probe {
		if !discoveryProbeHealthy(v) && !discoveryProbeTransitional(v) && v != "notAuthorized" {
			t.Errorf("discovery probe state %q is classified by neither healthy, transitional, nor notAuthorized", v)
		}
	}
	if !containsStr(probe, "notAuthorized") {
		t.Error("discovery enum no longer contains 'notAuthorized'; the gap detector is dead")
	}

	sev := enumOf(fAlertSeverity.Type, fAlertSeverity.Field)
	if !containsStr(sev, "critical") || !containsStr(sev, "warning") {
		t.Errorf("alert severity enum changed unexpectedly: %v", sev)
	}
}

// TestNoDismissalTimestampExists documents, as an executable assertion, WHY
// `alert noise` reports no mean-time-to-dismiss. If Auvik ever adds a dismissal
// timestamp this fails, which is the signal to restore the metric.
func TestNoDismissalTimestampExists(t *testing.T) {
	doc := loadSpecDoc(t)
	typ := doc.Types[fAlertDismissed.Type]
	for _, f := range typ.Fields {
		switch f.Name {
		case "dismissedOn", "dismissedTime", "dismissedAt", "dismissedDate":
			t.Fatalf("alertAttributes now declares %q — alert noise CAN compute "+
				"mean-time-to-dismiss again; restore the metric", f.Name)
		}
	}
}

// TestNoConfigBodyExists documents why `configuration grep` does not exist and
// `configuration audit` took its place. Auvik's Configuration API returns backup
// METADATA only. If a body field ever appears, a real fleet-wide config grep
// becomes possible and this fails to say so.
func TestNoConfigBodyExists(t *testing.T) {
	doc := loadSpecDoc(t)
	typ := doc.Types[fConfigBackupTime.Type]
	for _, f := range typ.Fields {
		switch f.Name {
		case "configuration", "configContents", "contents", "body", "text", "config":
			t.Fatalf("configAttributes now declares %q — a real fleet-wide "+
				"`configuration grep` is now possible; reconsider the audit-only command", f.Name)
		}
	}
	// Positive assertion: the two metadata fields the audit command depends on.
	var haveBackup, haveRunning bool
	for _, f := range typ.Fields {
		if f.Name == fConfigBackupTime.Field {
			haveBackup = true
		}
		if f.Name == fConfigIsRunning.Field {
			haveRunning = true
		}
	}
	if !haveBackup || !haveRunning {
		t.Fatalf("configAttributes lost backupTime/isRunning; `configuration audit` is broken")
	}
}

func containsStr(list []string, want string) bool {
	for _, v := range list {
		if v == want {
			return true
		}
	}
	return false
}
