// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// cli-printing-press: novel-scaffold-test
// Novel command scaffold tests. Keep the wiring smoke test and add behavior cases as needed.

package cli

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"cork-pp-cli/internal/cliutil/testenv"
)

// TestNovelScoreAttributeHelpWires smoke-tests that the score attribute command
// resolves at runtime and renders useful --help output. Catches wiring
// regressions (missing AddCommand, panicking RunE on --help, etc.) before
// review. Keep this smoke test when adding behavior-specific cases.
func TestNovelScoreAttributeHelpWires(t *testing.T) {
	testenv.Isolate(t)
	cmd := RootCmd()
	cmd.SetArgs([]string{"score", "attribute", "--help"})
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	if err := cmd.Execute(); err != nil {
		t.Fatalf("score attribute --help error = %v (novel command not wired correctly?)", err)
	}
	help := out.String()
	for _, want := range []string{"Usage:", "attribute"} {
		if !strings.Contains(help, want) {
			t.Fatalf("score attribute --help missing %q in output:\n%s", want, help)
		}
	}
}

// Every in-window point is 600; the client was 800 until just before the window
// opened. The move is 800 -> 600, not 600 -> 600.
func TestScoreAttributeUsesScoreAtWindowOpen(t *testing.T) {
	now := time.Now()
	ts := func(d time.Duration) string { return now.Add(-d).UTC().Format(time.RFC3339) }
	pt := func(score int, d time.Duration, comp float64) map[string]any {
		return map[string]any{"score": score, "created_at": ts(d), "compliance_impact": comp}
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/clients/c-1/score-history", func(w http.ResponseWriter, r *http.Request) {
		items := []any{pt(600, 6*time.Hour, 200), pt(600, 30*time.Hour, 200)}
		if r.URL.Query().Get("created_before") != "" {
			items = []any{pt(800, 8*24*time.Hour, 0), pt(790, 9*24*time.Hour, 5)}
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"items": items, "pagination": map[string]any{"page": 1, "total_pages": 1}})
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	t.Setenv("CORK_BASE_URL", srv.URL)
	t.Setenv("CORK_API_KEY", "dummy")
	cmd := RootCmd()
	cmd.SetArgs([]string{"score", "attribute", "c-1", "--since", "7d", "--json"})
	var out, errOut bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&errOut)
	_ = cmd.Execute()
	var v scoreAttributeView
	s := out.String()
	if i := strings.Index(s, "{"); i < 0 || json.Unmarshal([]byte(s[i:]), &v) != nil {
		t.Fatalf("no JSON view:\n%s\n%s", s, errOut.String())
	}
	if v.ScoreThen != 800 || v.ScoreNow != 600 || v.Delta != -200 {
		t.Fatalf("want 800 -> 600 (-200), got %d -> %d (%+d):\n%s", v.ScoreThen, v.ScoreNow, v.Delta, s)
	}
}
