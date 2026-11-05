// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"kaseya-bms-pp-cli/internal/config"
	"kaseya-bms-pp-cli/internal/store"
)

// kbmsListPage answers one page of a Kaseya BMS list the way the API does:
// {"Success":true,"Error":null,"Result":[...],"TotalRecords":N}, the records
// of the page PageNumber asks for (1-based; absent is page 1) at the size
// PageSize asks for, capped at limit whatever it asks. A page past the end
// is an empty Result inside the envelope.
func kbmsListPage(w http.ResponseWriter, r *http.Request, total, limit int, record func(i int) map[string]any) {
	q := r.URL.Query()
	page, err := strconv.Atoi(q.Get("PageNumber"))
	if err != nil || page < 1 {
		page = 1
	}
	size := limit
	if m, err := strconv.Atoi(q.Get("PageSize")); err == nil && m > 0 && m < size {
		size = m
	}
	items := []map[string]any{}
	for i := (page - 1) * size; i < total && i < page*size; i++ {
		items = append(items, record(i))
	}
	_ = json.NewEncoder(w).Encode(map[string]any{"Success": true, "Error": nil, "Result": items, "TotalRecords": total})
}

// Codex r1 (action1 reprint on 4.34.2) class incomplete paging read as
// complete, on Kaseya BMS's list shape: every list is one page of records in
// the envelope, paged by PageNumber (1-based) and PageSize (sync asks 100).
// Each list holds more records than one page (tickets, a list that ends on a
// full page so only an empty page proves the end, and opportunities; every
// record keyed Id), so sync must walk every page to store them all, on a
// token the BMS login mints.
func TestSyncReadsEveryPage(t *testing.T) {
	inHoldingChild(t)
	// The smallest counts that keep each shape: every stored record pays the
	// kit's scan, and this test cost 399 s of the package's ten minutes in the
	// Linux row at 263, 200 and 151.
	const tickets, timelogs, opportunities, pageCap = 101, 100, 101, 100
	login := kbmsLoginFor("paging")
	var mints atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodPost && r.URL.Path == kbmsAuthPath:
			if !readKbmsMint(r).valid(login) {
				w.WriteHeader(http.StatusUnauthorized)
				_, _ = io.WriteString(w, kbmsLoginRefused)
				return
			}
			mints.Add(1)
			_, _ = io.WriteString(w, kbmsTokenAnswer("paging-minted-token-6c1e", "paging-refresh-6c1e", time.Now().Add(time.Hour)))
		case r.Header.Get("Authorization") != "Bearer paging-minted-token-6c1e":
			w.WriteHeader(http.StatusUnauthorized)
		case r.URL.Path == "/v2/servicedesk/tickets":
			kbmsListPage(w, r, tickets, pageCap, func(i int) map[string]any {
				return map[string]any{"Id": 5000 + i, "TicketNumber": fmt.Sprintf("T20261029.%04d", i), "Title": fmt.Sprintf("Ticket %d", i), "StatusName": "New"}
			})
		case r.URL.Path == "/v2/timelogs":
			kbmsListPage(w, r, timelogs, pageCap, func(i int) map[string]any {
				return map[string]any{"Id": 7000 + i, "TicketId": 5000 + i, "Timespent": 30, "IsBillable": true, "AccountName": "Acme"}
			})
		case r.URL.Path == "/v2/crm/opportunities":
			kbmsListPage(w, r, opportunities, pageCap, func(i int) map[string]any {
				return map[string]any{"Id": 9000 + i, "Name": fmt.Sprintf("Opportunity %d", i), "StageName": "Proposal"}
			})
		default:
			_, _ = io.WriteString(w, `{"Success":true,"Error":null,"Result":[],"TotalRecords":0}`)
		}
	}))
	defer srv.Close()
	dir := t.TempDir()
	for k, v := range map[string]string{"KASEYA_BMS_HOME": dir, "HOME": dir, "XDG_CONFIG_HOME": filepath.Join(dir, "config"),
		"XDG_DATA_HOME": filepath.Join(dir, "data"), "XDG_CACHE_HOME": filepath.Join(dir, "cache"), "KASEYA_BMS_TOKEN": "",
		"KASEYA_BMS_BEARER_AUTH": "", "KASEYA_BMS_CONFIG": filepath.Join(dir, "config.toml"), "KASEYA_BMS_BASE_URL": srv.URL,
		"KASEYA_BMS_USERNAME": login.Username, "KASEYA_BMS_PASSWORD": login.Password, "KASEYA_BMS_TENANT": login.Tenant} {
		t.Setenv(k, v)
	}
	db := filepath.Join(dir, "tickets.db")
	var out, errb bytes.Buffer
	cmd := newRootCmd(&rootFlags{})
	cmd.SetOut(&out)
	cmd.SetErr(&errb)
	cmd.SetArgs([]string{"sync", "--db", db, "--full", "--resources", "servicedesk,timelogs,crm", "--json"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("sync: %v\nstdout: %s\nstderr: %s", err, out.String(), errb.String())
	}
	if mints.Load() == 0 {
		t.Fatalf("sync never minted; the fake answered nothing:\n%s%s", out.String(), errb.String())
	}
	s, err := store.Open(db)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	for rt, want := range map[string]int{"servicedesk": tickets, "timelogs": timelogs, "crm": opportunities} {
		var n int
		if err := s.DB().QueryRow(`SELECT count(*) FROM resources WHERE resource_type = ?`, rt).Scan(&n); err != nil {
			t.Fatal(err)
		}
		if n != want {
			t.Errorf("%s: sync stored %d of %d records; it stopped before the last page:\n%s", rt, n, want, strings.TrimSpace(errb.String()))
		}
	}
}

// kbmsLoginFor is a BMS login whose every part names tag.
func kbmsLoginFor(tag string) config.KbmsLogin {
	return config.KbmsLogin{Username: tag + "-api-user", Password: tag + "-api-password-3b7d", Tenant: tag + "-tenant"}
}

// Codex r1 (action1 reprint on 4.34.2) class rotating token changes offline
// store identity, as it reaches Kaseya BMS: each region host is its own
// tenant. Host A was used with a pasted token (auth set-token) and synced;
// the operator then points the CLI at host B with B's login. The default
// store is named by the token in effect when no login is configured and must
// never be adopted into the login's store on the next run, so B's ticket
// views (stale-tickets, read from the local store) show only what B synced.
func TestStoreOfAnotherHostIsNeverAdopted(t *testing.T) {
	inHoldingChild(t)
	lastActivity := time.Now().Add(-40 * 24 * time.Hour).UTC().Format(time.RFC3339)
	host := func(tag string) *httptest.Server {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			switch {
			case r.Method == http.MethodPost && r.URL.Path == kbmsAuthPath:
				_, _ = io.WriteString(w, kbmsTokenAnswer("scope-minted-"+tag+"-4d2a", "scope-refresh-"+tag, time.Now().Add(time.Hour)))
			case r.URL.Path == "/v2/servicedesk/tickets":
				_, _ = io.WriteString(w, `{"Success":true,"Error":null,"Result":[`+
					`{"Id":7`+fmt.Sprint(len(tag))+`,"TicketNumber":"T-`+tag+`","Title":"Ticket-`+tag+`","AccountName":"Account-`+tag+`",`+
					`"StatusName":"New","QueueName":"Help Desk","LastActivityUpdate":"`+lastActivity+`"}],"TotalRecords":1}`)
			default:
				_, _ = io.WriteString(w, `{"Success":true,"Error":null,"Result":[],"TotalRecords":0}`)
			}
		}))
		t.Cleanup(srv.Close)
		return srv
	}
	a, b := host("HostA"), host("HostB")
	dir := t.TempDir()
	for k, v := range map[string]string{"KASEYA_BMS_HOME": dir, "HOME": dir, "XDG_CONFIG_HOME": filepath.Join(dir, "config"),
		"XDG_DATA_HOME": filepath.Join(dir, "data"), "XDG_CACHE_HOME": filepath.Join(dir, "cache"), "KASEYA_BMS_CONFIG": "",
		"KASEYA_BMS_TOKEN": "", "KASEYA_BMS_BEARER_AUTH": "", "KASEYA_BMS_USERNAME": "", "KASEYA_BMS_PASSWORD": "", "KASEYA_BMS_TENANT": ""} {
		t.Setenv(k, v)
	}
	run := func(stdin string, args ...string) string {
		t.Helper()
		var out bytes.Buffer
		cmd := newRootCmd(&rootFlags{})
		cmd.SetIn(strings.NewReader(stdin))
		cmd.SetOut(&out)
		cmd.SetErr(&out)
		cmd.SetArgs(args)
		if err := cmd.Execute(); err != nil {
			t.Fatalf("%v: %v\n%s", args, err, out.String())
		}
		return out.String()
	}
	t.Setenv("KASEYA_BMS_BASE_URL", a.URL)
	run("scope-pasted-token-HostA-0123456789\n", "auth", "set-token")
	if out := run("", "sync", "--resources", "servicedesk", "--json"); !strings.Contains(out, `"total_records": 1`) && !strings.Contains(out, `"total_records":1`) {
		t.Fatalf("host A's sync stored nothing; the test did not run:\n%s", out)
	}
	if out := run("", "stale-tickets", "--days", "1", "--json"); !strings.Contains(out, "T-HostA") {
		t.Fatalf("host A's own view lost its ticket; the control did not run:\n%s", out)
	}
	t.Setenv("KASEYA_BMS_BASE_URL", b.URL)
	login := kbmsLoginFor("scope-HostB")
	t.Setenv("KASEYA_BMS_USERNAME", login.Username)
	t.Setenv("KASEYA_BMS_PASSWORD", login.Password)
	t.Setenv("KASEYA_BMS_TENANT", login.Tenant)
	if out := run("", "stale-tickets", "--days", "1", "--json"); strings.Contains(out, "T-HostA") {
		t.Errorf("host B's view reads host A's synced ticket (A's store was adopted into B's):\n%s", out)
	}
	if out := run("", "sync", "--resources", "servicedesk", "--json"); !strings.Contains(out, `"errored": 0`) && !strings.Contains(out, `"errored":0`) {
		t.Fatalf("host B's sync failed:\n%s", out)
	}
	out := run("", "stale-tickets", "--days", "1", "--json")
	if !strings.Contains(out, "T-HostB") {
		t.Fatalf("host B's view lost its own ticket; the control did not run:\n%s", out)
	}
	if strings.Contains(out, "T-HostA") {
		t.Errorf("host B's view reads host A's synced ticket:\n%s", out)
	}
}

// export reads a Kaseya BMS list the way the API pages it (PageNumber from
// 1, PageSize 100, until a short or empty page) and writes each record, not
// each page envelope, then stops. The fake refuses after a page budget, so a
// runaway fails fast instead of hanging the test.
func TestExportWalksEveryKbmsPageAndStops(t *testing.T) {
	inHoldingChild(t)
	const timelogs = 263
	var requests atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if requests.Add(1) > 20 {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = io.WriteString(w, `{"Success":false,"Error":{"Code":400,"Message":"page budget exhausted"},"Result":null}`)
			return
		}
		if r.URL.Path != "/v2/timelogs" || r.Header.Get("Authorization") != "Bearer export-token-5f1a" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		kbmsListPage(w, r, timelogs, 100, func(i int) map[string]any {
			return map[string]any{"Id": 900 + i, "TicketId": 50 + i, "Notes": fmt.Sprintf("Time entry %d", i), "IsBillable": true}
		})
	}))
	defer srv.Close()
	dir := t.TempDir()
	for k, v := range map[string]string{"KASEYA_BMS_HOME": dir, "HOME": dir, "XDG_CONFIG_HOME": filepath.Join(dir, "config"),
		"XDG_DATA_HOME": filepath.Join(dir, "data"), "XDG_CACHE_HOME": filepath.Join(dir, "cache"), "KASEYA_BMS_CONFIG": "",
		"KASEYA_BMS_USERNAME": "", "KASEYA_BMS_PASSWORD": "", "KASEYA_BMS_TENANT": "", "KASEYA_BMS_BEARER_AUTH": "",
		"KASEYA_BMS_BASE_URL": srv.URL, "KASEYA_BMS_TOKEN": "export-token-5f1a"} {
		t.Setenv(k, v)
	}
	for _, format := range []string{"jsonl", "json"} {
		requests.Store(0)
		out := filepath.Join(dir, "timelogs."+format)
		var stdout, stderr bytes.Buffer
		cmd := newRootCmd(&rootFlags{})
		cmd.SetOut(&stdout)
		cmd.SetErr(&stderr)
		cmd.SetArgs([]string{"export", "timelogs", "--format", format, "--output", out})
		if err := cmd.Execute(); err != nil {
			t.Fatalf("export --format %s: %v (%d requests)\n%s", format, err, requests.Load(), stderr.String())
		}
		raw, err := os.ReadFile(out)
		if err != nil {
			t.Fatal(err)
		}
		var records []map[string]any
		if format == "json" {
			if err := json.Unmarshal(raw, &records); err != nil {
				t.Fatalf("export --format json is not a JSON array: %v\n%.500s", err, raw)
			}
		} else {
			for _, line := range strings.Split(strings.TrimSpace(string(raw)), "\n") {
				var rec map[string]any
				if err := json.Unmarshal([]byte(line), &rec); err != nil {
					t.Fatalf("export line is not a JSON object: %v: %.200s", err, line)
				}
				records = append(records, rec)
			}
		}
		if len(records) != timelogs {
			t.Fatalf("--format %s: exported %d records of %d in %d requests", format, len(records), timelogs, requests.Load())
		}
		if fmt.Sprint(records[0]["Id"]) != "900" || fmt.Sprint(records[timelogs-1]["Id"]) != fmt.Sprint(900+timelogs-1) {
			t.Fatalf("--format %s: want Id 900 .. %d, got %v .. %v (a page skipped, or envelopes written)", format, 900+timelogs-1, records[0]["Id"], records[timelogs-1]["Id"])
		}
		if n := requests.Load(); n > 4 {
			t.Fatalf("--format %s: %d requests for a three-page list", format, n)
		}
	}
}
