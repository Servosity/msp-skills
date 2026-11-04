// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

// Class test (mirror-freshness): every table an answer joins, and the parent
// it was walked over, is judged for age before a count from it is presented
// as complete. The Codex round-1 input is `fleet missed --window 24h
// --max-age 1m` over users and devices synced 48 hours ago: it reported
// registered_devices:0 and complete:true.

package cli

import (
	"net/http"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"netsapiens-pp-cli/internal/cliutil/testenv"
)

// seedAgedCovered leaves rt as a completed, unfiltered sync that started a
// minute before at.
func seedAgedCovered(t *testing.T, path, rt string, at time.Time, docs ...string) {
	t.Helper()
	seedSyncedRows(t, path, rt, at, docs...)
	seedSyncState(t, path, rt, true, at)
	seedProvenance(t, path, rt, at.Add(-time.Minute), true)
}

func TestClassEveryJoinedDependencyIsJudgedForAge(t *testing.T) {
	now := time.Now().UTC()
	old := now.Add(-48 * time.Hour)
	const user = `{"domain":"acme.example","user":"100"}`
	const device = `{"domain":"acme.example","device":"100a","user":"100","device-sip-registration-state":"unregistered","device-provisioning-mac-address":"0004f2aabbcc"}`
	missedCDRs := func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/cdrs") {
			writeJSON(w, `[{"domain":"acme.example","call-term-user":"100","call-orig-call-id":"c1","call-start-datetime":"`+now.Format(time.RFC3339)+`"}]`)
			return
		}
		writeJSON(w, `[]`)
	}
	missed := []string{"fleet", "missed", "--domain", "acme.example", "--window", "24h", "--max-age", "10m", "--no-cache", "--json"}
	cases := []struct {
		name  string
		seed  func(t *testing.T, path string)
		args  []string
		stale []string
	}{
		{"fleet missed: users and devices older than --max-age", func(t *testing.T, path string) {
			seedAgedCovered(t, path, "domains", old.Add(-time.Hour), `{"domain":"acme.example"}`)
			seedAgedCovered(t, path, "users", old, user)
			seedAgedCovered(t, path, "devices", old, device)
		}, missed, []string{"users", "devices"}},
		{"fleet missed: fresh users and devices walked over a stale domain list", func(t *testing.T, path string) {
			seedAgedCovered(t, path, "domains", old, `{"domain":"acme.example"}`)
			seedAgedCovered(t, path, "users", now, user)
			seedAgedCovered(t, path, "devices", now, device)
		}, missed, []string{"domains"}},
		{"trace: a matched number's attached phone model comes from a stale NDP table", func(t *testing.T, path string) {
			seedAgedCovered(t, path, "smsnumbers", now)
			seedAgedCovered(t, path, "domains_phones", now)
			seedAgedCovered(t, path, "domains", now, `{"domain":"acme.example"}`)
			seedAgedCovered(t, path, "users", now, user)
			seedAgedCovered(t, path, "devices", now, device)
			seedAgedCovered(t, path, "phonenumbers", now, `{"domain":"acme.example","phonenumber":"16025550100","dial-rule-application":"to-user","dial-rule-translation-destination-user":"100","dial-rule-translation-destination-host":"acme.example"}`)
			seedAgedCovered(t, path, "phones", old, `{"domain":"acme.example","device-provisioning-mac-address":"0004f2aabbcc","device-models-brand-and-model":"Old Model"}`)
		}, []string{"trace", "16025550100", "--max-age", "10m", "--json"}, []string{"phones"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			testenv.Isolate(t)
			fakeNetsapiens(t, missedCDRs)
			path := filepath.Join(t.TempDir(), "data.db")
			tc.seed(t, path)
			out := mustRunFleet(t, append(tc.args, "--db", path)...)
			rep := decodeObject(t, out)
			if rep["complete"] == true {
				t.Fatalf("a joined table older than --max-age (or walked over a stale parent) read as complete:\n%s", out)
			}
			for _, rt := range tc.stale {
				if !strings.Contains(stringsOf(rep["stale_resources"]), rt) {
					t.Fatalf("stale_resources must name %s:\n%s", rt, out)
				}
			}
		})
	}
}
