// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.
// cli-printing-press: novel-scaffold-test
// Novel command scaffold tests. Keep the wiring smoke test and add behavior cases as needed.

package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"saas-alerts-pp-cli/internal/cliutil/testenv"
)

// TestNovelRuleTriggersQueueHelpWires smoke-tests that the rule-triggers queue command
// resolves at runtime and renders useful --help output. Catches wiring
// regressions (missing AddCommand, panicking RunE on --help, etc.) before
// review. Keep this smoke test when adding behavior-specific cases.
func TestNovelRuleTriggersQueueHelpWires(t *testing.T) {
	testenv.Isolate(t)
	cmd := RootCmd()
	cmd.SetArgs([]string{"rule-triggers", "queue", "--help"})
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	if err := cmd.Execute(); err != nil {
		t.Fatalf("rule-triggers queue --help error = %v (novel command not wired correctly?)", err)
	}
	help := out.String()
	for _, want := range []string{"Usage:", "queue"} {
		if !strings.Contains(help, want) {
			t.Fatalf("rule-triggers queue --help missing %q in output:\n%s", want, help)
		}
	}
}

func rqtQueueIDs(env rqQueueEnvelope) []string {
	ids := make([]string, 0, len(env.Items))
	for _, it := range env.Items {
		ids = append(ids, it.TriggerID)
	}
	return ids
}

func TestRuleTriggersQueueLiveOrderingAndPaging(t *testing.T) {
	api := newRQTAPI(t)
	rqtEnv(t, api)
	out, stderr, err := rqtRun(t, "rule-triggers", "queue", "--json")
	if err != nil {
		t.Fatalf("queue: %v\nstderr: %s", err, stderr)
	}
	env := rqtDecode[rqQueueEnvelope](t, out)
	if env.Source != "live" || env.Pages != 2 || env.Truncated {
		t.Fatalf("source/pages/truncated = %s/%d/%v, want live/2/false", env.Source, env.Pages, env.Truncated)
	}
	if got, want := strings.Join(rqtQueueIDs(env), ","), "RT_C,RT_B,RT_D,RT_A2,RT_E,RT_A"; got != want {
		t.Fatalf("order = %s, want %s", got, want)
	}
	if env.Fetched != 6 || env.ExpiredCount != 2 {
		t.Fatalf("fetched/expired_count = %d/%d, want 6/2", env.Fetched, env.ExpiredCount)
	}
	first := env.Items[0]
	if first.DeadlineKind != "auto_ignore_at" || *first.TimeLeft != 900 || *first.RemindersSent != 2 ||
		*first.AutoIgnoreAt != "2026-10-14T12:15:00Z" || first.OrganizationName != "Fabrikam" ||
		strings.Join(first.WouldFire, ",") != "Block Sign In" || strings.Join(first.Accounts, ",") != "carol@example.com" {
		t.Fatalf("RT_C computed wrong: %+v", first)
	}
	last := env.Items[5]
	if !last.Expired || !last.Superseded || last.ExpiryReason != "superseded" || *last.ExpiresAt != "2026-10-14T11:00:00Z" {
		t.Fatalf("RT_A should be the superseded, expired last item: %+v", last)
	}

	lists := api.callsMatching("GET", "/respondApi/api/v1/rule-triggers")
	if len(lists) != 2 {
		t.Fatalf("list calls = %d, want 2 pages", len(lists))
	}
	if got := strings.Join(lists[0].Query["statuses[]"], ","); got != "PENDING,REMEDIATION_FAILED" {
		t.Fatalf("statuses[] = %q", got)
	}
	if len(lists[0].Query["startAfter"]) != 0 {
		t.Fatalf("first page sent a cursor: %v", lists[0].Query)
	}
	wantCursor := fmt.Sprintf("%d,RT_C", rqtMS(-40*time.Minute))
	if got := strings.Join(lists[1].Query["startAfter"], ","); got != wantCursor {
		t.Fatalf("second page startAfter = %q, want %q", got, wantCursor)
	}
	if len(api.writes()) != 0 {
		t.Fatalf("queue is read-only but wrote: %+v", api.writes())
	}
}

func TestRuleTriggersQueueFilters(t *testing.T) {
	api := newRQTAPI(t)
	rqtEnv(t, api)

	out, _, err := rqtRun(t, "rule-triggers", "queue", "--expiring-within", "1h", "--json")
	if err != nil {
		t.Fatalf("queue --expiring-within: %v", err)
	}
	env := rqtDecode[rqQueueEnvelope](t, out)
	if got := strings.Join(rqtQueueIDs(env), ","); got != "RT_C,RT_B" {
		t.Fatalf("--expiring-within 1h = %s, want RT_C,RT_B", got)
	}
	if env.ExpiredCount != 2 {
		t.Fatalf("expired_count should still count hidden expired triggers, got %d", env.ExpiredCount)
	}

	out, _, err = rqtRun(t, "rule-triggers", "queue", "--rule", "RULE_DOES_NOT_EXIST", "--json")
	if err != nil {
		t.Fatalf("queue --rule: %v", err)
	}
	env = rqtDecode[rqQueueEnvelope](t, out)
	if len(env.Items) != 0 || env.Fetched != 0 || env.ExpiredCount != 0 {
		t.Fatalf("unmatched rule returned items: %+v", env)
	}
	if !strings.Contains(out, `"items": []`) && !strings.Contains(out, `"items":[]`) {
		t.Fatalf("empty items must serialize as [], got %s", out)
	}

	out, _, err = rqtRun(t, "rule-triggers", "queue", "--org", "ORG_2", "--json")
	if err != nil {
		t.Fatalf("queue --org: %v", err)
	}
	env = rqtDecode[rqQueueEnvelope](t, out)
	if got := strings.Join(rqtQueueIDs(env), ","); got != "RT_C,RT_E" {
		t.Fatalf("--org ORG_2 = %s, want RT_C,RT_E", got)
	}

	out, stderr, err := rqtRun(t, "rule-triggers", "queue", "--max-pages", "1", "--json")
	if err != nil {
		t.Fatalf("queue --max-pages 1: %v", err)
	}
	env = rqtDecode[rqQueueEnvelope](t, out)
	if !env.Truncated || env.Pages != 1 || env.Fetched != 3 || !strings.Contains(env.TruncatedWhy, "--max-pages 1") || !strings.Contains(stderr, "queue is incomplete") {
		t.Fatalf("--max-pages 1: truncated=%v pages=%d fetched=%d stderr=%q", env.Truncated, env.Pages, env.Fetched, stderr)
	}

	_, _, err = rqtRun(t, "rule-triggers", "queue", "--statuses", "BOGUS")
	if rqtExit(err) != 2 {
		t.Fatalf("unknown status exit = %d (%v), want 2", rqtExit(err), err)
	}
	_, _, err = rqtRun(t, "rule-triggers", "queue", "--expiring-within", "soon")
	if rqtExit(err) != 2 {
		t.Fatalf("bad duration exit = %d (%v), want 2", rqtExit(err), err)
	}
}

func TestRuleTriggersQueueSelectAndAgent(t *testing.T) {
	api := newRQTAPI(t)
	rqtEnv(t, api)
	out, _, err := rqtRun(t, "rule-triggers", "queue", "--json", "--select", "items.trigger_id,items.time_left_seconds,expired_count")
	if err != nil {
		t.Fatalf("queue --select: %v", err)
	}
	var sel map[string]any
	if err := json.Unmarshal([]byte(out), &sel); err != nil {
		t.Fatalf("select output: %v\n%s", err, out)
	}
	items, _ := sel["items"].([]any)
	if len(items) != 6 || sel["expired_count"] != float64(2) {
		t.Fatalf("--select output wrong: %s", out)
	}
	firstItem, _ := items[0].(map[string]any)
	if firstItem["trigger_id"] != "RT_C" || firstItem["time_left_seconds"] != float64(900) || firstItem["rule_name"] != nil {
		t.Fatalf("--select kept wrong fields: %v", firstItem)
	}

	out, _, err = rqtRun(t, "rule-triggers", "queue", "--agent")
	if err != nil {
		t.Fatalf("queue --agent: %v", err)
	}
	var agent struct {
		Meta    map[string]any  `json:"meta"`
		Results rqQueueEnvelope `json:"results"`
	}
	if err := json.Unmarshal([]byte(out), &agent); err != nil {
		t.Fatalf("agent output: %v\n%s", err, out)
	}
	if agent.Meta["source"] != "live" || len(agent.Results.Items) != 6 {
		t.Fatalf("agent envelope wrong: %s", out)
	}
	if it := agent.Results.Items[0]; it.TriggerID != "RT_C" || it.Deadline == nil || it.TimeLeft == nil || len(it.WouldFire) == 0 {
		t.Fatalf("--agent compaction dropped queue fields: %+v", it)
	}
}

func TestRuleTriggersQueueLocalFallback(t *testing.T) {
	api := newRQTAPI(t)
	rqtEnv(t, api)
	dbPath := rqtSeedStore(t, rqtTriggers(), nil)

	out, _, err := rqtRun(t, "rule-triggers", "queue", "--data-source", "local", "--db", dbPath, "--json")
	if err != nil {
		t.Fatalf("queue local: %v", err)
	}
	env := rqtDecode[rqQueueEnvelope](t, out)
	if env.Source != "local" {
		t.Fatalf("source = %q, want local", env.Source)
	}
	if got := strings.Join(rqtQueueIDs(env), ","); got != "RT_C,RT_B,RT_D,RT_A2,RT_E,RT_A" {
		t.Fatalf("local order = %s", got)
	}
	if api.callCount() != 0 {
		t.Fatalf("--data-source local made %d API calls", api.callCount())
	}

	// auto with no API key: live is unavailable, the mirror answers.
	t.Setenv("SAAS_ALERTS_API_KEY", "")
	out, _, err = rqtRun(t, "rule-triggers", "queue", "--db", dbPath, "--json")
	if err != nil {
		t.Fatalf("queue auto without key: %v", err)
	}
	env = rqtDecode[rqQueueEnvelope](t, out)
	if env.Source != "local" || !strings.Contains(env.FallbackReason, "no API key") || len(env.Items) != 6 {
		t.Fatalf("no-key fallback wrong: source=%s reason=%q items=%d", env.Source, env.FallbackReason, len(env.Items))
	}
	if api.callCount() != 0 {
		t.Fatalf("no-key fallback still called the API %d times", api.callCount())
	}
	out, _, err = rqtRun(t, "rule-triggers", "queue", "--db", dbPath, "--agent")
	if err != nil {
		t.Fatalf("queue auto --agent without key: %v", err)
	}
	var agent struct {
		Meta    map[string]any  `json:"meta"`
		Results rqQueueEnvelope `json:"results"`
	}
	if err := json.Unmarshal([]byte(out), &agent); err != nil {
		t.Fatalf("agent output: %v\n%s", err, out)
	}
	if agent.Meta["source"] != "local" || agent.Results.Source != "local" {
		t.Fatalf("agent meta should report local source: %s", out)
	}

	// auto with the API unreachable (connection refused) also falls back.
	dead := httptest.NewServer(http.NotFoundHandler())
	deadURL := dead.URL
	dead.Close()
	t.Setenv("SAAS_ALERTS_API_KEY", rqtAPIKey)
	t.Setenv("SAAS_ALERTS_BASE_URL", deadURL)
	out, stderr, err := rqtRun(t, "rule-triggers", "queue", "--db", dbPath, "--json")
	if err != nil {
		t.Fatalf("queue auto with dead API: %v", err)
	}
	env = rqtDecode[rqQueueEnvelope](t, out)
	if env.Source != "local" || len(env.Items) != 6 || !strings.Contains(stderr, "showing the local mirror") {
		t.Fatalf("network fallback wrong: source=%s items=%d stderr=%q", env.Source, len(env.Items), stderr)
	}

	// --data-source live never falls back.
	_, _, err = rqtRun(t, "rule-triggers", "queue", "--data-source", "live", "--db", dbPath, "--json")
	if err == nil {
		t.Fatalf("--data-source live with a dead API should fail")
	}
}

func TestRuleTriggersQueueMissingMirror(t *testing.T) {
	rqtEnv(t, nil)
	missing := filepath.Join(t.TempDir(), "nope.db")
	out, stderr, err := rqtRun(t, "rule-triggers", "queue", "--data-source", "local", "--db", missing, "--json")
	if err != nil {
		t.Fatalf("missing mirror should exit 0, got %v", err)
	}
	env := rqtDecode[rqQueueEnvelope](t, out)
	if env.Source != "local" || len(env.Items) != 0 || env.Items == nil {
		t.Fatalf("missing mirror envelope wrong: %s", out)
	}
	if !strings.Contains(stderr, "no local mirror at "+missing) || !strings.Contains(stderr, "sync --resources rule-triggers") {
		t.Fatalf("missing mirror hint absent: %q", stderr)
	}
}

func TestRuleTriggersQueueEnvelopeAndCursorFailures(t *testing.T) {
	rtB := rqtTriggers()[1]
	rtA2 := rqtTriggers()[0]
	cases := []struct {
		name      string
		responses []any
		wantErr   string
		truncated string
		fetched   int
	}{
		{name: "error-shaped 200 object is an error", responses: []any{map[string]any{"message": "backend failure"}},
			wantErr: "answered with a message field"},
		{name: "ruleTriggers that is not an array is an error", responses: []any{map[string]any{"ruleTriggers": "nope"}},
			wantErr: "not an array"},
		{name: "a top-level null body is an error", responses: []any{nil},
			wantErr: "expected an object with ruleTriggers, got null"},
		{name: "ruleTriggers null is an error", responses: []any{map[string]any{"ruleTriggers": nil, "pagination": map[string]any{"hasNextPage": false}}},
			wantErr: "ruleTriggers is not an array"},
		{name: "hasNextPage without a cursor is truncation", fetched: 1, truncated: "cursor is missing",
			responses: []any{map[string]any{"ruleTriggers": []any{rtB}, "pagination": map[string]any{"hasNextPage": true}}}},
		{name: "hasNextPage with a malformed cursor is truncation", fetched: 1, truncated: "cursor is missing or not",
			responses: []any{map[string]any{"ruleTriggers": []any{rtB}, "pagination": map[string]any{"hasNextPage": true, "cursor": "abc"}}}},
		{name: "a repeated cursor is truncation and does not duplicate triggers", fetched: 1, truncated: "repeated a pagination cursor",
			responses: []any{map[string]any{"ruleTriggers": []any{rtA2}, "pagination": map[string]any{"hasNextPage": true, "cursor": []any{rtA2["triggeredAt"], "RT_A2"}}}}},
		{name: "hasNextPage null is truncation", fetched: 1, truncated: rqTruncPaginationFlag,
			responses: []any{map[string]any{"ruleTriggers": []any{rtB}, "pagination": map[string]any{"hasNextPage": nil, "cursor": []any{1, "RT_B"}}}}},
		{name: "hasNextPage absent is truncation", fetched: 1, truncated: rqTruncPaginationFlag,
			responses: []any{map[string]any{"ruleTriggers": []any{rtB}, "pagination": map[string]any{"cursor": []any{1, "RT_B"}}}}},
		{name: "hasNextPage as a string is truncation", fetched: 1, truncated: rqTruncPaginationFlag,
			responses: []any{map[string]any{"ruleTriggers": []any{rtB}, "pagination": map[string]any{"hasNextPage": "yes", "cursor": []any{1, "RT_B"}}}}},
		{name: "no pagination object is truncation", fetched: 1, truncated: rqTruncPaginationFlag,
			responses: []any{map[string]any{"ruleTriggers": []any{rtB}}}},
		{name: "an empty page that claims more is truncation", fetched: 0, truncated: "empty page",
			responses: []any{map[string]any{"ruleTriggers": []any{}, "pagination": map[string]any{"hasNextPage": true, "cursor": []any{1, "X"}}}}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			api := newRQTAPI(t)
			api.listResponses = tc.responses
			rqtEnv(t, api)
			out, _, err := rqtRun(t, "rule-triggers", "queue", "--json")
			if tc.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("want error containing %q, got err=%v out=%s", tc.wantErr, err, out)
				}
				if strings.Contains(out, `"items"`) {
					t.Fatalf("a failed read must not print a queue: %s", out)
				}
				return
			}
			if err != nil {
				t.Fatalf("queue: %v", err)
			}
			env := rqtDecode[rqQueueEnvelope](t, out)
			if !env.Truncated || !strings.Contains(env.TruncatedWhy, tc.truncated) || env.Fetched != tc.fetched {
				t.Fatalf("truncated=%v reason=%q fetched=%d; want true, %q, %d", env.Truncated, env.TruncatedWhy, env.Fetched, tc.truncated, tc.fetched)
			}
		})
	}
}

func TestRuleTriggersQueueSecondsFields(t *testing.T) {
	api := newRQTAPI(t)
	rqtEnv(t, api)
	out, _, err := rqtRun(t, "rule-triggers", "queue", "--expiring-within", "1h", "--json")
	if err != nil {
		t.Fatalf("queue: %v", err)
	}
	var raw map[string]any
	if err := json.Unmarshal([]byte(out), &raw); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if raw["expiring_within_seconds"] != float64(3600) {
		t.Fatalf("expiring_within_seconds = %v, want 3600", raw["expiring_within_seconds"])
	}
	if _, old := raw["expiring_within"]; old {
		t.Fatalf("the Go duration string field is still emitted")
	}
	item := raw["items"].([]any)[0].(map[string]any)
	if item["time_left_seconds"] != float64(900) {
		t.Fatalf("time_left_seconds = %v, want 900", item["time_left_seconds"])
	}
	if _, old := item["time_left"]; old {
		t.Fatalf("time_left is still emitted")
	}

	out, _, err = rqtRun(t, "rule-triggers", "queue", "--json")
	if err != nil {
		t.Fatalf("queue: %v", err)
	}
	if strings.Contains(out, "expiring_within_seconds") {
		t.Fatalf("expiring_within_seconds must be omitted when --expiring-within is unset")
	}
}

// rqtMCPHintLines applies the MCP shell-out's stderr rule
// (stderrHintLines in internal/mcp/cobratree/shellout.go): only lines that
// start with "hint:" or "warning:" reach the agent.
func rqtMCPHintLines(stderr string) []string {
	kept := make([]string, 0)
	for _, raw := range strings.Split(stderr, "\n") {
		for _, seg := range strings.Split(raw, "\r") {
			line := strings.TrimSpace(seg)
			lower := strings.ToLower(line)
			if strings.HasPrefix(lower, "hint:") || strings.HasPrefix(lower, "warning:") {
				kept = append(kept, line)
			}
		}
	}
	return kept
}

func TestRuleTriggersQueueUnsyncedLocalResultIsMarked(t *testing.T) {
	rqtEnv(t, nil)
	missing := filepath.Join(t.TempDir(), "nope.db")
	out, stderr, err := rqtRun(t, "rule-triggers", "queue", "--data-source", "local", "--db", missing, "--json")
	if err != nil {
		t.Fatalf("missing mirror: %v", err)
	}
	env := rqtDecode[rqQueueEnvelope](t, out)
	if env.Synced == nil || *env.Synced || !strings.Contains(env.Note, "no local mirror") || len(env.Items) != 0 || env.Source != "local" {
		t.Fatalf("missing mirror must be marked synced:false with a note: %s", out)
	}
	kept := strings.Join(rqtMCPHintLines(stderr), "\n")
	if !strings.Contains(kept, "no local mirror at "+missing) || !strings.Contains(kept, "sync --resources rule-triggers --db "+missing) {
		t.Fatalf("the sync guidance does not survive the MCP stderr filter:\nstderr=%q\nkept=%q", stderr, kept)
	}

	// A store that exists but was never synced for rule-triggers.
	empty := rqtSeedStore(t, nil, nil)
	out, _, err = rqtRun(t, "rule-triggers", "queue", "--data-source", "local", "--db", empty, "--json")
	if err != nil {
		t.Fatalf("unsynced store: %v", err)
	}
	if env := rqtDecode[rqQueueEnvelope](t, out); env.Synced == nil || *env.Synced || !strings.Contains(env.Note, "never been synced") {
		t.Fatalf("an unsynced store must be marked synced:false: %s", out)
	}

	// A synced store reports synced:true and no note.
	synced := rqtSeedStore(t, rqtTriggers(), nil)
	out, _, err = rqtRun(t, "rule-triggers", "queue", "--data-source", "local", "--db", synced, "--json")
	if err != nil {
		t.Fatalf("synced store: %v", err)
	}
	if env := rqtDecode[rqQueueEnvelope](t, out); env.Synced == nil || !*env.Synced || env.Note != "" || len(env.Items) == 0 {
		t.Fatalf("a synced store must be marked synced:true: %s", out)
	}
}
