// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package msgraphblackbox

import (
	"bytes"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// alertsPager answers GET /v1.0/security/alerts_v2 the way the adversarial
// review's stub did (Graph paging: "The requested page size might be
// ignored"): pages of 50 whatever $top asks, an @odata.nextLink on every page
// but the last, 120 alerts in all. Every other collection is empty.
type alertsPager struct {
	nextHost atomic.Value // origin the nextLink names; "" = this server
	requests atomic.Int64
}

func (p *alertsPager) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	p.requests.Add(1)
	w.Header().Set("Content-Type", "application/json")
	if r.URL.Path != "/v1.0/security/alerts_v2" {
		_, _ = w.Write([]byte(`{"value":[]}`))
		return
	}
	const total, page = 120, 50
	start, _ := strconv.Atoi(r.URL.Query().Get("$skiptoken"))
	recent := time.Now().UTC().Add(-time.Hour).Format(time.RFC3339)
	items := []map[string]any{}
	for i := start; i < total && i < start+page; i++ {
		items = append(items, map[string]any{"id": fmt.Sprintf("alert-%04d", i), "title": fmt.Sprintf("alert %d", i),
			"status": "new", "severity": "high", "createdDateTime": recent})
	}
	body := map[string]any{"value": items}
	if start+page < total {
		host, _ := p.nextHost.Load().(string)
		if host == "" {
			host = "https://" + r.Host
		}
		q := url.Values{"$top": {r.URL.Query().Get("$top")}, "$skiptoken": {strconv.Itoa(start + page)}}
		body["@odata.nextLink"] = host + r.URL.Path + "?" + q.Encode()
	}
	_ = json.NewEncoder(w).Encode(body)
}

// runExit runs the CLI and returns its combined output and exit code.
func (e *bbEnv) runExit(t *testing.T, args ...string) (string, int) {
	t.Helper()
	cmd := exec.Command(bbCLI, args...)
	cmd.Env, cmd.Dir = e.env, e.home
	var out bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &out
	err := cmd.Run()
	var exit *exec.ExitError
	if errors.As(err, &exit) {
		return out.String(), exit.ExitCode()
	}
	if err != nil {
		t.Fatalf("%v: %v", args, err)
	}
	return out.String(), 0
}

// mcpResults maps each tools/call id to its result text and isError flag.
func mcpResults(t *testing.T, session string) map[int]string {
	t.Helper()
	out := map[int]string{}
	for _, line := range strings.Split(session, "\n") {
		var m struct {
			ID     *int `json:"id"`
			Result struct {
				IsError bool `json:"isError"`
				Content []struct {
					Text string `json:"text"`
				} `json:"content"`
			} `json:"result"`
		}
		if json.Unmarshal([]byte(line), &m) != nil || m.ID == nil {
			continue
		}
		var texts []string
		for _, c := range m.Result.Content {
			texts = append(texts, c.Text)
		}
		out[*m.ID] = fmt.Sprintf("isError=%v %s", m.Result.IsError, strings.Join(texts, " | "))
	}
	return out
}

// microsoft-graph handfix (sync-reads-every-page), class
// incomplete-read-as-complete, through the built binaries: the CLI's sync and
// the MCP sync and workflow_archive tools store and report all 120 alerts,
// and a nextLink naming another host is an error, never followed, never
// sync_complete.
func TestSyncBinaryReadsEveryPage(t *testing.T) {
	var offHost atomic.Int64
	other := tlsServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		offHost.Add(1)
		_, _ = w.Write([]byte(`{"value":[]}`))
	}))
	pager := &alertsPager{}
	srv := tlsServer(t, pager)
	ca := filepath.Join(t.TempDir(), "ca.pem")
	if err := os.WriteFile(ca, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: srv.Certificate().Raw}), 0o600); err != nil {
		t.Fatal(err)
	}
	sv := &server{url: srv.URL + "/v1.0", caFile: ca}

	e := newEnv(t, sv, true)
	out, code := e.runExit(t, "sync", "--json", "--resources", "security")
	t.Logf("CLI sync --json --resources security (exit %d):\n%s", code, out)
	if code != 0 || !strings.Contains(out, `{"event":"sync_complete","resource":"security","total":120,`) {
		t.Errorf("CLI sync: exit %d, want 0 and sync_complete total 120", code)
	}
	out, code = e.runExit(t, "security", "triage", "--since", "7d", "--json")
	t.Logf("CLI security triage --since 7d --json (exit %d):\n%.600s", code, out)
	if code != 0 || !strings.Contains(out, `"totalOpen": 120`) {
		t.Errorf("security triage after sync: exit %d, want totalOpen 120", code)
	}

	count := map[string]any{"name": "sql", "arguments": map[string]any{"query": "SELECT COUNT(*) AS n FROM resources WHERE resource_type = 'security'"}}
	m := newEnv(t, sv, true)
	got := mcpResults(t, m.mcpPhases(t,
		[]map[string]any{{"name": "sync", "arguments": map[string]any{"resources": "security"}}},
		[]map[string]any{count},
		[]map[string]any{{"name": "security_triage", "arguments": map[string]any{"since": "7d"}}},
	))
	a := newEnv(t, sv, true)
	archived := mcpResults(t, a.mcpPhases(t,
		[]map[string]any{{"name": "workflow_archive", "arguments": map[string]any{}}},
		[]map[string]any{count},
	))
	for _, id := range []int{1, 2, 3} {
		t.Logf("MCP sync session call %d: %.400s", id, got[id])
	}
	for _, id := range []int{1, 2} {
		t.Logf("MCP workflow_archive session call %d: %.400s", id, archived[id])
	}
	if !strings.Contains(got[1], `"resource":"security","total":120`) && !strings.Contains(got[1], `\"resource\":\"security\",\"total\":120`) && !strings.Contains(got[1], `"total_records": 120`) {
		t.Errorf("MCP sync did not report 120 alerts: %.600s", got[1])
	}
	if !strings.Contains(got[2], `"n": 120`) && !strings.Contains(got[2], `"n":120`) {
		t.Errorf("MCP sql after sync: %.300s, want 120 stored alerts", got[2])
	}
	if !strings.Contains(got[3], `"totalOpen": 120`) && !strings.Contains(got[3], `"totalOpen":120`) {
		t.Errorf("MCP security_triage after sync: %.300s, want totalOpen 120", got[3])
	}
	if !strings.Contains(archived[2], `"n": 120`) && !strings.Contains(archived[2], `"n":120`) {
		t.Errorf("MCP sql after workflow_archive: %.300s, want 120 stored alerts", archived[2])
	}

	// A nextLink naming another host: the CLI and the MCP tool report an
	// error, store nothing of the unfinished read, and never contact it.
	pager.nextHost.Store(other.URL)
	f := newEnv(t, sv, true)
	out, code = f.runExit(t, "sync", "--json", "--resources", "security")
	t.Logf("CLI sync, nextLink to another host (exit %d):\n%s", code, out)
	if code == 0 || strings.Contains(out, `"event":"sync_complete","resource":"security"`) || !strings.Contains(out, `"event":"sync_error","resource":"security"`) {
		t.Errorf("CLI sync over a foreign nextLink: exit %d, want non-zero with sync_error and no sync_complete", code)
	}
	fm := newEnv(t, sv, true)
	foreign := mcpResults(t, fm.mcpPhases(t,
		[]map[string]any{{"name": "sync", "arguments": map[string]any{"resources": "security"}}},
		[]map[string]any{count},
	))
	t.Logf("MCP sync, nextLink to another host: %.600s", foreign[1])
	if !strings.HasPrefix(foreign[1], "isError=true") || strings.Contains(foreign[1], `"resource":"security","total":`) {
		t.Errorf("MCP sync over a foreign nextLink: %.600s, want an error and no sync_complete", foreign[1])
	}
	if !strings.Contains(foreign[2], `"n": 0`) && !strings.Contains(foreign[2], `"n":0`) {
		t.Errorf("MCP sql after the failed sync: %.300s, want 0 stored alerts", foreign[2])
	}
	if n := offHost.Load(); n != 0 {
		t.Errorf("the other host was contacted %d time(s)", n)
	}
}
