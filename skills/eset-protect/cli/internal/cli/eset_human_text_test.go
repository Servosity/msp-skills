// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"eset-protect-pp-cli/internal/cliutil/testenv"
	"eset-protect-pp-cli/internal/store"
)

// lineSpoofSuffix is the adversarial round-2 receipt: an embedded LF that
// forged a second preview line, plus a tab that would shift a column.
const lineSpoofSuffix = "\nPREVIEW (nothing sent): StartNetworkIsolation on HARMLESS-KIOSK\tFORGED-COLUMN"

func seedLineSpoofMirror(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "linespoof.db")
	db, err := store.OpenWithContext(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	now := time.Now().UTC()
	put := func(resource, id string, v map[string]any) {
		raw, _ := json.Marshal(v)
		if err := db.Upsert(resource, id, raw); err != nil {
			t.Fatal(err)
		}
	}
	put("device-groups", "msp", map[string]any{"uuid": "msp", "displayName": "MSP", "linkedEntityType": "DEVICE_GROUP_ENTITY_TYPE_MSP"})
	put("device-groups", "c1", map[string]any{"uuid": "c1", "displayName": "Acme" + lineSpoofSuffix, "parentGroupUuid": "msp", "linkedEntityType": esetCompanyEntityType})
	put("device-groups", "c2", map[string]any{"uuid": "c2", "displayName": "Acme Two" + lineSpoofSuffix, "parentGroupUuid": "msp", "linkedEntityType": esetCompanyEntityType})
	dev := "00000000-0000-0000-0000-000000000003"
	put("devices", dev, map[string]any{"uuid": dev, "displayName": "DC01" + lineSpoofSuffix, "parentGroupUuid": "c1",
		"functionalityStatus": "DEVICE_FUNCTIONALITY_STATUS_ATTENTION_REQUIRED", "lastSyncTime": now.Add(-40 * 24 * time.Hour).Format(time.RFC3339)})
	put("detections", "det-1", map[string]any{"uuid": "det-1", "displayName": "DC01" + lineSpoofSuffix, "objectName": "DC01" + lineSpoofSuffix,
		"objectHashSha1": strings.Repeat("a", 40), "occurTime": now.Add(-time.Hour).Format(time.RFC3339), "severityLevel": "SEVERITY_LEVEL_HIGH", "context": map[string]any{"deviceUuid": dev}})
	put("incidents", "inc-1", map[string]any{"uuid": "inc-1", "displayName": "DC01" + lineSpoofSuffix, "severity": "INCIDENT_SEVERITY_LEVEL_HIGH",
		"status": "INCIDENT_STATUS_OPEN", "createTime": now.Add(-time.Hour).Format(time.RFC3339), "deviceUuids": []string{dev}})
	for _, r := range []string{"devices", "device-groups", "detections", "incidents"} {
		if err := db.SaveSyncState(r, "", 1); err != nil {
			t.Fatal(err)
		}
	}
	return path
}

// TestHumanPrintersFlattenTenantLines guards human-printers-flatten-tenant-lines:
// a tenant name carrying LF or tab cannot add a line or a column to a
// hand-written human printer. The respond isolate preview (the line a human
// approves before --yes) prints exactly one PREVIEW line, every line naming
// HARMLESS-KIOSK also names the real DC01, and no tenant tab survives. JSON
// keeps the name byte for byte (encoding/json escapes it).
func TestHumanPrintersFlattenTenantLines(t *testing.T) {
	testenv.Isolate(t)
	respondClearHarness(t)
	respondFakeESET(t, func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("no ESET call expected, got %s %s", r.Method, r.URL.Path)
		w.WriteHeader(http.StatusTeapot)
	})
	db := seedLineSpoofMirror(t)
	for _, argv := range [][]string{
		{"respond", "isolate", "00000000-0000-0000-0000-000000000003", "--db", db},
		{"fleet", "health", "--db", db},
		{"fleet", "stale", "--db", db},
		{"fleet", "gaps", "--db", db},
		{"triage", "--since", "1w", "--db", db},
		{"detections", "spread", "--since", "1w", "--db", db},
		// The generated analytics table (adversarial round 3, P2).
		{"analytics", "--type", "devices", "--group-by", "displayName", "--db", db},
		{"analytics", "--type", "detections", "--group-by", "objectName", "--db", db},
	} {
		out, errOut, err := captureExecuteAll(t, append(argv, "--human-friendly")...)
		if err != nil {
			t.Fatalf("%v: %v\n%s\n%s", argv, err, out, errOut)
		}
		for _, line := range strings.Split(string(out), "\n") {
			if strings.Contains(line, "HARMLESS-KIOSK") && !strings.Contains(line, "DC01") && !strings.Contains(line, "Acme") {
				t.Fatalf("%v: a tenant LF started a line of its own: %q\nfull output:\n%s", argv, line, out)
			}
			// The tenant's own tab (analytics separates its columns with a
			// tab of its own, after the value).
			if strings.Contains(line, "HARMLESS-KIOSK\tFORGED-COLUMN") {
				t.Fatalf("%v: a tenant tab survived: %q", argv, line)
			}
		}
		if argv[0] == "respond" {
			starts := 0
			for _, line := range strings.Split(string(out), "\n") {
				if strings.HasPrefix(line, "PREVIEW (nothing sent):") {
					starts++
				}
			}
			if starts != 1 || !strings.Contains(string(out), "PREVIEW (nothing sent): StartNetworkIsolation on DC01 PREVIEW (nothing sent): StartNetworkIsolation on HARMLESS-KIOSK FORGED-COLUMN\n") {
				t.Fatalf("the preview must print one PREVIEW line holding the whole name (got %d):\n%s", starts, out)
			}
		}
	}
	// JSON is unchanged: the name round-trips with its LF and tab.
	out, _, err := captureExecuteAll(t, "fleet", "health", "--db", db, "--json")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(out, []byte(`Acme\nPREVIEW (nothing sent): StartNetworkIsolation on HARMLESS-KIOSK\tFORGED-COLUMN`)) {
		t.Fatalf("JSON must keep the tenant name exactly: %s", out)
	}
	// The ambiguous-customer error lists both names, each on the error's line.
	_, errOut, err := captureExecuteAll(t, "fleet", "health", "--customer", "Acme", "--db", db)
	if err == nil || bytes.Contains(errOut, []byte("\nPREVIEW")) || !bytes.Contains(errOut, []byte("matches 2 customers")) {
		t.Fatalf("customer-resolution error must keep each name on one line: %v %q", err, errOut)
	}
}

func TestEsetFlattenAndHumanArgs(t *testing.T) {
	for in, want := range map[string]string{
		"a\nb":        "a b",
		"a\r\nb":      "a b",
		"a\rb\tc":     "a b c",
		"a\vb\fc":     "a b c",
		"a\u0085b":    "a b",
		"a\u2028b":    "a b",
		"a\u2029b":    "a b",
		"plain ascii": "plain ascii",
	} {
		if got := esetFlatten(in); got != want {
			t.Errorf("esetFlatten(%q) = %q, want %q", in, got, want)
		}
	}
	type named string
	got := esetHumanArgs([]any{"x\ny", []string{"a\tb"}, errors.New("e\nf"), 7, []byte("raw\n"), named("n\nm")})
	if got[0] != "x y" || got[1].([]string)[0] != "a b" || got[2] != "e f" || got[3] != 7 || string(got[4].([]byte)) != "raw\n" || got[5] != "n m" {
		t.Fatalf("esetHumanArgs = %#v", got)
	}
	var b bytes.Buffer
	_, _ = esetFprintf(&b, "%s\t%d\n", "name\nforged", 3)
	if b.String() != "name forged\t3\n" {
		t.Fatalf("the format keeps its own tab and newline, the argument is flattened: %q", b.String())
	}
}
