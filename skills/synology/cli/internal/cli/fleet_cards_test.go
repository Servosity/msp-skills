// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// HAND-AUTHORED. Pins the fleet/abb human view (writeFleetCards) and the
// null counts of fleet sync rows for anything not measured.

package cli

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"regexp"
	"strings"
	"testing"

	"synology-pp-cli/internal/dsm"
)

var cardLine = regexp.MustCompile(`^  ([a-z_]+):\s+(.*)$`)

type card struct {
	title string
	keys  []string
	vals  map[string]string
}

// parseCards splits writeFleetCards output into cards.
func parseCards(t *testing.T, out string) []card {
	t.Helper()
	var cards []card
	for _, block := range strings.Split(strings.TrimRight(out, "\n"), "\n\n") {
		lines := strings.Split(block, "\n")
		c := card{title: lines[0], vals: map[string]string{}}
		for _, l := range lines[1:] {
			if m := cardLine.FindStringSubmatch(l); m != nil {
				c.keys = append(c.keys, m[1])
				c.vals[m[1]] = m[2]
			}
		}
		cards = append(cards, c)
	}
	return cards
}

// checkCards: every card shows the same keys in the same order, identity
// before kind/state/ok before error/sync_error/note, and no value is cut.
func checkCards(t *testing.T, what string, cards []card) {
	t.Helper()
	if len(cards) == 0 {
		t.Fatalf("%s: no cards", what)
	}
	want := strings.Join(cards[0].keys, ",")
	for _, c := range cards {
		if got := strings.Join(c.keys, ","); got != want {
			t.Fatalf("%s: card %q shows keys %s, another shows %s (every row must show every key)", what, c.title, got, want)
		}
		for k, v := range c.vals {
			if strings.HasSuffix(v, "...") {
				t.Fatalf("%s: %s was truncated: %q", what, k, v)
			}
		}
	}
	tier := func(k string) int {
		switch k {
		case "nas":
			return 0
		case "client":
			return 1
		case "host_name", "task_name", "key":
			return 2
		case "kind", "state", "severity", "ok":
			return 3
		case "error", "sync_error", "last_sync_error", "problem", "note":
			return 4
		}
		return -1
	}
	last := -1
	for _, k := range cards[0].keys {
		tr := tier(k)
		if tr < 0 {
			continue
		}
		if tr < last {
			t.Fatalf("%s: key order %v puts %s after a later group", what, cards[0].keys, k)
		}
		last = tr
	}
}

// TestFleetHumanViewKeepsEveryColumn is the review's nul repro, in a
// terminal (--human-friendly is the TTY path): the generated table took its
// columns from the first row only, kept 6 chosen in map order (the NAS column
// came and went between runs of fleet sync), and cut values at 60
// characters, so a later row's error, an unknown NAS's sync_error and abb
// stale's nas and client were never shown. Every report now prints one card
// per row with every key any row carries, in one fixed order, uncut.
func TestFleetHumanViewKeepsEveryColumn(t *testing.T) {
	sweepIsolate(t)
	dsm.ForgetVerifiedAddresses()
	t.Setenv("NO_COLOR", "1")
	defer func(old bool) { humanFriendly = old }(humanFriendly)
	acme := fakeABBNAS(t)
	router, _ := notDSMHost(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = io.WriteString(w, "<html><body>router login</body></html>")
	})
	t.Setenv("SYNOLOGY_ACME_ACCOUNT", "msp-admin")
	t.Setenv("SYNOLOGY_ACME_PASSWORD", "s3cret")
	t.Setenv("SYNOLOGY_DELTA_ACCOUNT", "x")
	t.Setenv("SYNOLOGY_DELTA_PASSWORD", "PW-D-444444444")
	for _, add := range [][]string{
		{"fleet", "add", "acme", "--url", acme.URL, "--client", "Acme Corp"},
		{"fleet", "add", "delta", "--url", router, "--client", "Delta LLC"},
		{"fleet", "add", "eps", "--url", "http://127.0.0.1:9"},
	} {
		if out, errText, code := runCLIErr(t, add...); code != 0 {
			t.Fatalf("%v: exit %d\n%s%s", add, code, out, errText)
		}
	}
	var syncShapes []string
	for i := 0; i < 4; i++ {
		out, errText, code := runCLIErr(t, "fleet", "sync", "--human-friendly")
		if code != 5 {
			t.Fatalf("fleet sync: exit %d\n%s%s", code, out, errText)
		}
		cards := parseCards(t, out)
		checkCards(t, "fleet sync", cards)
		if len(cards) != 3 || cards[0].vals["nas"] != "acme" || cards[0].vals["client"] != "Acme Corp" || cards[0].vals["ok"] != "true" ||
			cards[0].vals["error"] != "-" || cards[0].vals["tasks"] != "2" ||
			cards[1].vals["nas"] != "delta" || cards[1].vals["client"] != "Delta LLC" || cards[1].vals["ok"] != "false" ||
			!strings.HasSuffix(cards[1].vals["error"], "the password was not sent - check SYNOLOGY_BASE_URL (or the fleet entry's --url)") ||
			cards[1].vals["tasks"] != "-" || cards[2].vals["nas"] != "eps" ||
			!strings.Contains(cards[2].vals["error"], `PowerShell: $env:SYNOLOGY_EPS_ACCOUNT = "<account>"`) {
			t.Fatalf("fleet sync human view:\n%s", out)
		}
		syncShapes = append(syncShapes, strings.Join(cards[0].keys, ","))
	}
	for _, s := range syncShapes[1:] {
		if s != syncShapes[0] {
			t.Fatalf("fleet sync's columns change between runs: %q vs %q", syncShapes[0], s)
		}
	}

	reports := []struct {
		args  []string
		check func(cards []card) bool
	}{
		{[]string{"fleet", "status"}, func(cs []card) bool {
			d := cs[1]
			return d.vals["nas"] == "delta" && d.vals["client"] == "Delta LLC" && d.vals["state"] == "unknown" &&
				strings.HasSuffix(d.vals["sync_error"], "the password was not sent - check SYNOLOGY_BASE_URL (or the fleet entry's --url)") &&
				d.vals["tasks"] == "-" && strings.Contains(cs[2].vals["sync_error"], "credentials not set")
		}},
		{[]string{"abb", "stale"}, func(cs []card) bool {
			for _, c := range cs {
				if c.vals["nas"] == "" || c.vals["client"] == "" || c.vals["kind"] == "" {
					return false
				}
			}
			return cs[0].vals["nas"] == "acme" && cs[0].vals["client"] == "Acme Corp" && cs[0].vals["host_name"] != "-" &&
				cs[len(cs)-1].vals["kind"] == "nas_not_synced" && strings.Contains(cs[len(cs)-1].vals["note"], "credentials not set")
		}},
		{[]string{"abb", "failures"}, func(cs []card) bool {
			return cs[0].vals["nas"] == "acme" && cs[0].vals["client"] == "Acme Corp" && cs[0].vals["task_name"] == "File Server" &&
				cs[0].vals["kind"] == "run_failed" && strings.Contains(cs[0].vals["first_error"], "fs_error=-65") &&
				cs[1].vals["kind"] == "nas_not_synced" && strings.HasSuffix(cs[1].vals["note"], "(or the fleet entry's --url)")
		}},
		{[]string{"abb", "trend", "--since", "30d"}, func(cs []card) bool {
			return cs[0].vals["nas"] == "acme" && cs[0].vals["client"] == "Acme Corp" && cs[0].vals["key"] != "-" &&
				cs[len(cs)-1].vals["kind"] == "nas_not_synced" && cs[len(cs)-1].vals["runs"] == "-" &&
				strings.Contains(cs[len(cs)-1].vals["note"], "credentials not set: set SYNOLOGY_EPS_ACCOUNT and SYNOLOGY_EPS_PASSWORD")
		}},
		{[]string{"fleet", "health"}, func(cs []card) bool {
			for _, c := range cs {
				if c.vals["nas"] == "acme" && c.vals["client"] == "Acme Corp" && c.vals["kind"] == "volume" && c.vals["problem"] != "-" {
					return strings.HasSuffix(cs[0].vals["problem"], "(or the fleet entry's --url)")
				}
			}
			return false
		}},
		{[]string{"fleet", "list"}, func(cs []card) bool {
			return cs[0].keys[0] == "name" && cs[0].vals["name"] == "acme" && cs[1].vals["client"] == "Delta LLC" &&
				strings.HasSuffix(cs[1].vals["last_sync_error"], "(or the fleet entry's --url)") && cs[2].vals["tls"] != ""
		}},
		{[]string{"abb", "device", "FS01"}, func(cs []card) bool {
			return cs[0].vals["nas"] == "acme" && cs[0].vals["client"] == "Acme Corp" && cs[0].vals["host_name"] == "FS01"
		}},
	}
	for _, r := range reports {
		args := append(append([]string{}, r.args...), "--human-friendly")
		first, errText, _ := runCLIErr(t, args...)
		for i := 0; i < 3; i++ {
			again, _, _ := runCLIErr(t, args...)
			if again != first {
				t.Fatalf("%v is not deterministic:\n%s\n---\n%s", r.args, first, again)
			}
		}
		cards := parseCards(t, first)
		checkCards(t, strings.Join(r.args, " "), cards)
		if !r.check(cards) {
			t.Fatalf("%v human view:\n%s%s", r.args, first, errText)
		}
	}
}

// TestFleetCardsUnionAndOrder pins writeFleetCards on rows whose first row
// lacks the explanatory fields a later row carries.
func TestFleetCardsUnionAndOrder(t *testing.T) {
	type row struct {
		Count  *int   `json:"count"`
		NAS    string `json:"nas"`
		Note   string `json:"note,omitempty"`
		OK     bool   `json:"ok"`
		Client string `json:"client,omitempty"`
		Error  string `json:"error,omitempty"`
		Kind   string `json:"kind"`
		Host   string `json:"host_name,omitempty"`
	}
	two := 2
	long := strings.Repeat("a very long explanation ", 20) + "END"
	var buf bytes.Buffer
	rows := []row{{Count: &two, NAS: "acme", OK: true, Kind: "task"}, {NAS: "delta", Client: "Delta", Error: long, Kind: "nas_not_synced", Note: "see error", Host: "SRV1"}}
	if err := writeFleetCards(&buf, rows, "empty"); err != nil {
		t.Fatal(err)
	}
	cards := parseCards(t, buf.String())
	if got := strings.Join(cards[0].keys, ","); got != "nas,client,host_name,kind,ok,error,note,count" {
		t.Fatalf("key order %s\n%s", got, buf.String())
	}
	checkCards(t, "synthetic", cards)
	if cards[0].vals["error"] != "-" || cards[0].vals["client"] != "-" || cards[0].vals["host_name"] != "-" || cards[1].title != "delta / Delta / SRV1" || cards[1].vals["count"] != "-" || cards[0].vals["count"] != "2" ||
		cards[1].vals["error"] != long || cards[0].vals["ok"] != "true" || cards[1].vals["ok"] != "false" {
		t.Fatalf("values:\n%s", buf.String())
	}
	buf.Reset()
	if err := writeFleetCards(&buf, []row{}, "nothing to show"); err != nil || buf.String() != "nothing to show\n" {
		t.Fatalf("empty: %q %v", buf.String(), err)
	}
}

// TestFleetSyncCountsAreNullWhenNotMeasured is the review's noabb repro: a
// NAS without Active Backup for Business synced storage only, yet its row
// said tasks/results/device_runs 0 while fleet status said null. Each count
// is now null unless that part was measured; a measured zero stays 0.
func TestFleetSyncCountsAreNullWhenNotMeasured(t *testing.T) {
	cases := []struct {
		name string
		body map[string]string
		want map[string]any // nil = null
	}{
		{"no ABB", map[string]string{
			"SYNO.Core.System/info":              `{"model":"DS920+","firmware_ver":"DSM 7.2.2"}`,
			"SYNO.Storage.CGI.Storage/load_info": `{"volumes":[{"id":"volume_1","vol_path":"/volume1","status":"normal","size":{"total":"1000","used":"10"}}],"storagePools":[],"disks":[]}`,
		}, map[string]any{"tasks": nil, "results": nil, "device_runs": nil, "storage_items": float64(1), "abb_installed": false}},
		{"ABB, no storage", map[string]string{
			"SYNO.Core.System/info":                                `{"model":"DS920+","firmware_ver":"DSM 7.2.2"}`,
			"SYNO.ActiveBackup.Task/list":                          `{"tasks":[{"task_id":8,"task_name":"File Server","devices":[]}]}`,
			"SYNO.ActiveBackup.Log/list_result":                    `{"results":[]}`,
			"SYNO.ActiveBackup.Overview/list_device_transfer_size": `{"device_list":[]}`,
		}, map[string]any{"tasks": float64(1), "results": float64(0), "device_runs": float64(0), "storage_items": nil, "abb_installed": true}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			sweepIsolate(t)
			dsm.ForgetVerifiedAddresses()
			nas := newStubNAS(t, tc.body)
			t.Setenv("SYNOLOGY_BASE_URL", nas.URL)
			t.Setenv("SYNOLOGY_ACCOUNT", "admin")
			t.Setenv("SYNOLOGY_PASSWORD", "pw-null-1")
			out, errText, _ := runCLIErr(t, "fleet", "sync", "--json")
			var rows []map[string]any
			if err := json.Unmarshal([]byte(out), &rows); err != nil || len(rows) != 1 {
				t.Fatalf("fleet sync: %v\n%s%s", err, out, errText)
			}
			for k, want := range tc.want {
				got, present := rows[0][k]
				if !present || got != want {
					t.Errorf("%s = %v (present %v), want %v\n%s", k, got, present, want, out)
				}
			}
		})
	}
}
