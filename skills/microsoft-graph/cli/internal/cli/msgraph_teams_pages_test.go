// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"microsoft-graph-pp-cli/internal/cliutil/testenv"
)

// microsoft-graph handfix (teams-phone-reads-every-page), class
// incomplete-read-as-complete. The fake pages the way Microsoft Learn documents
// it: numberAssignments ("List numberAssignments": $top default 500, at most
// 1,000, and $skip) and userConfigurations (@odata.nextLink).
type teamsPagesFake struct {
	numbers    int  // numbers in the tenant; the last quarter are unassigned
	ignoreSkip bool // a server that answers every $skip with the first page
	requests   []string
}

func (f *teamsPagesFake) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.requests = append(f.requests, r.URL.RequestURI())
	w.Header().Set("Content-Type", "application/json")
	q := r.URL.Query()
	switch strings.TrimPrefix(r.URL.Path, "/v1.0") {
	case "/admin/teams/telephoneNumberManagement/numberAssignments":
		top, skip := 500, 0
		if v := q.Get("$top"); v != "" {
			top, _ = strconv.Atoi(v)
		}
		if top > 1000 {
			top = 1000
		}
		if v := q.Get("$skip"); v != "" && !f.ignoreSkip {
			skip, _ = strconv.Atoi(v)
		}
		page := []map[string]any{} // Graph sends an empty array, never null
		for i := skip; i < f.numbers && i < skip+top; i++ {
			status := "userAssigned"
			if i >= f.numbers*3/4 {
				status = "unassigned"
			}
			page = append(page, map[string]any{"id": fmt.Sprintf("num-%d", i), "telephoneNumber": fmt.Sprintf("+1425555%04d", i), "assignmentStatus": status})
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"value": page})
	case "/admin/teams/userConfigurations":
		if q.Get("$skiptoken") == "PAGE2" {
			_, _ = w.Write([]byte(`{"value":[{"id":"usr-2","isEnterpriseVoiceEnabled":true}]}`))
			return
		}
		next := "http://" + r.Host + "/v1.0/admin/teams/userConfigurations?$skiptoken=PAGE2"
		_, _ = fmt.Fprintf(w, `{"@odata.nextLink":%q,"value":[{"id":"usr-1","isEnterpriseVoiceEnabled":true}]}`, next)
	default:
		w.WriteHeader(http.StatusNotFound)
	}
}

func teamsResults(t *testing.T, out string) []map[string]any {
	t.Helper()
	var env struct {
		Results []map[string]any `json:"results"`
	}
	if err := json.Unmarshal([]byte(out[strings.Index(out, "{"):]), &env); err != nil {
		t.Fatalf("output is not the JSON envelope: %v\n%s", err, out)
	}
	return env.Results
}

func TestTeamsPhoneListsReadEveryPage(t *testing.T) {
	testenv.Isolate(t)
	f := &teamsPagesFake{numbers: 1600}
	srv := httptest.NewServer(f)
	t.Cleanup(srv.Close)
	t.Setenv("MICROSOFT_GRAPH_BASE_URL", srv.URL+"/v1.0")
	t.Setenv("MICROSOFT_GRAPH_TOKEN", "teamsPagesToken-7c2a")

	out, err := runTranscripts(t, "phone-numbers", "list", "--json")
	if err != nil {
		t.Fatalf("phone-numbers list: %v\n%s", err, out)
	}
	got := teamsResults(t, out)
	unassigned := 0
	for _, n := range got {
		if n["assignmentStatus"] == "unassigned" {
			unassigned++
		}
	}
	if len(got) != 1600 || unassigned != 400 {
		t.Errorf("phone-numbers list read %d numbers (%d unassigned), want 1600 (400 unassigned); requests %v", len(got), unassigned, f.requests)
	}

	// --skip is where the read starts; it still reads to the end.
	out, err = runTranscripts(t, "phone-numbers", "list", "--skip", "1500", "--json")
	if err != nil || len(teamsResults(t, out)) != 100 {
		t.Errorf("phone-numbers list --skip 1500: %v, want the last 100 numbers\n%s", err, out)
	}

	out, err = runTranscripts(t, "teams-users", "list", "--json")
	if err != nil {
		t.Fatalf("teams-users list: %v\n%s", err, out)
	}
	if got := teamsResults(t, out); len(got) != 2 {
		t.Errorf("teams-users list read %d users, want both nextLink pages", len(got))
	}

	db := filepath.Join(t.TempDir(), "state.db")
	out, err = runTranscripts(t, "pull", "--db", db, "--json", "--only", "phone-numbers")
	if err != nil || !strings.Contains(out, `"count": 1600`) && !strings.Contains(out, `"count":1600`) {
		t.Errorf("pull --only phone-numbers: %v, want 1600 stored\n%s", err, out)
	}

	// A server that ignores $skip must fail the read, never repeat or stop short.
	f.ignoreSkip = true
	if out, err := runTranscripts(t, "phone-numbers", "list", "--json"); err == nil {
		t.Errorf("phone-numbers list exited 0 against a server that ignores $skip:\n%s", out)
	}
}
