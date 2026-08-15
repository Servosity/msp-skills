// Copyright 2026 geekbrownbear and contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// corkStub serves the minimum Cork surface `coverage gaps` reads, with
// connectorCount connectors so a default --max-connectors of 10 truncates.
func corkStub(t *testing.T, connectorCount int) *httptest.Server {
	t.Helper()
	const client = "11111111-1111-1111-1111-111111111111"
	const tenant = "aaaaaaaa-0000-0000-0000-00000000000a"

	send := func(w http.ResponseWriter, v any) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(v)
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/clients", func(w http.ResponseWriter, r *http.Request) {
		send(w, map[string]any{"items": []any{map[string]any{
			"uuid": client, "name": "Acme Corp",
			"associated_tenants": []any{map[string]any{"uuid": tenant, "name": "Acme tenant"}},
		}}})
	})
	mux.HandleFunc("/clients/"+client+"/devices", func(w http.ResponseWriter, r *http.Request) {
		send(w, map[string]any{"items": []any{map[string]any{
			"uuid": "d1", "hostname": "acme-laptop-1",
			"associated_endpoints": []any{map[string]any{"integration_identifier": "dev-owned-1"}},
		}}})
	})
	mux.HandleFunc("/integrations/connected", func(w http.ResponseWriter, r *http.Request) {
		items := make([]any, 0, connectorCount)
		for i := 0; i < connectorCount; i++ {
			items = append(items, map[string]any{
				"uuid": fmt.Sprintf("%08d-0000-0000-0000-000000000000", i),
				"name": fmt.Sprintf("connector-%d", i), "display_name": fmt.Sprintf("connector-%d", i),
				"connection_status": "ok", "last_synced_at": "2026-08-15T00:00:00Z",
				"associated_tenants": []any{map[string]any{"uuid": tenant, "name": "Acme tenant"}},
			})
		}
		send(w, map[string]any{"items": items})
	})
	// Every connector reports one device the client is NOT attributed -> a gap.
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		p := r.URL.Path
		if strings.HasPrefix(p, "/integrations/") && strings.HasSuffix(p, "/devices") {
			id := strings.TrimSuffix(strings.TrimPrefix(p, "/integrations/"), "/devices")
			send(w, map[string]any{"items": []any{map[string]any{
				"uuid": "cd-" + id, "hostname": "unseen-" + id,
				"integration_identifier": "dev-gap-" + id,
				"tenant":                 map[string]any{"uuid": tenant},
				"tenant_uuid":            tenant,
			}}})
			return
		}
		send(w, map[string]any{"items": []any{}})
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func runCoverageGaps(t *testing.T, srv *httptest.Server, args ...string) string {
	t.Helper()
	t.Setenv("CORK_BASE_URL", srv.URL)
	t.Setenv("CORK_API_KEY", "dummy")
	cmd := RootCmd()
	cmd.SetArgs(args)
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	_ = cmd.Execute()
	return out.String()
}

// TestCoverageGapsWarnsWhenSweepTruncatedWithGapsPresent is the regression test
// for the false all-clear the README promises against: with 21 connectors and a
// cap of 10, the gap list is a floor rather than a total, and saying "N gap(s)
// across 10 connector(s)" without that caveat reads as a complete answer.
// The zero-gap path always carried the caveat; the rows-present path did not.
func TestCoverageGapsWarnsWhenSweepTruncatedWithGapsPresent(t *testing.T) {
	srv := corkStub(t, 21)
	out := runCoverageGaps(t, srv,
		"coverage", "gaps",
		"--client", "11111111-1111-1111-1111-111111111111",
		"--db", t.TempDir()+"/absent.db",
		"--max-connectors", "10",
		"--human-friendly", // the human table is the surface that dropped the caveat
	)
	if !strings.Contains(out, "gap(s)") {
		t.Fatalf("expected a gap table, got:\n%s", out)
	}
	if !strings.Contains(out, "truncated") {
		t.Fatalf("truncated sweep reported no caveat - this is the false all-clear.\nOutput:\n%s", out)
	}
}

// TestCoverageGapsSilentWhenSweepComplete keeps the warning honest: a sweep that
// read every connector must NOT print a truncation caveat, or the caveat becomes
// noise operators learn to ignore.
func TestCoverageGapsSilentWhenSweepComplete(t *testing.T) {
	srv := corkStub(t, 3)
	out := runCoverageGaps(t, srv,
		"coverage", "gaps",
		"--client", "11111111-1111-1111-1111-111111111111",
		"--db", t.TempDir()+"/absent.db",
		"--max-connectors", "10",
		"--human-friendly",
	)
	if strings.Contains(out, "truncated") {
		t.Fatalf("complete sweep wrongly reported truncation:\n%s", out)
	}
}

// TestCoverageTruncationCauseMatchesItsRemedy pins the pairing. Three different
// conditions set scanCapHit and they do NOT share a remedy: no cap can make a
// malformed device decode, so sending the operator to raise one is a remedy that
// cannot change the outcome.
func TestCoverageTruncationCauseMatchesItsRemedy(t *testing.T) {
	for _, tc := range []struct {
		name                             string
		skipped, undecodable             int
		wantCause, wantRemedy            string
	}{
		{"connector cap", 11, 0, "11 connector(s) skipped or failed", "raise --max-connectors or --max-scan-pages"},
		{"page cap", 0, 0, "a page limit was reached", "raise --max-connectors or --max-scan-pages"},
		{"undecodable devices", 0, 3, "3 connector device(s) could not be decoded and were not diffed", "data-shape problem"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cause, remedy := coverageTruncationCause(tc.skipped, tc.undecodable)
			if !strings.Contains(cause, tc.wantCause) {
				t.Fatalf("cause = %q, want it to contain %q", cause, tc.wantCause)
			}
			if !strings.Contains(remedy, tc.wantRemedy) {
				t.Fatalf("remedy = %q, want it to contain %q", remedy, tc.wantRemedy)
			}
		})
	}
	// The undecodable remedy must NOT send the operator to a cap.
	_, remedy := coverageTruncationCause(0, 1)
	if strings.Contains(remedy, "--max-connectors") || strings.Contains(remedy, "--max-scan-pages") {
		t.Fatalf("undecodable remedy points at a cap that cannot fix it: %q", remedy)
	}
}
