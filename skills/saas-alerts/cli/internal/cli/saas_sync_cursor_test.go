// Hand-written for saas-alerts (not generated): rule-trigger sync walks the
// two-element pagination.cursor via repeated startAfter keys (hand-fix H3).

package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"

	"saas-alerts-pp-cli/internal/cliutil/testenv"
	"saas-alerts-pp-cli/internal/store"
)

func TestSaasCursorValueJoinsArrayCursor(t *testing.T) {
	if got := cursorValue(json.RawMessage(`[1686299747517,"NEXT_ID"]`)); got != "1686299747517,NEXT_ID" {
		t.Fatalf("cursorValue array = %q", got)
	}
	if got := cursorValue(json.RawMessage(`[1686299747517,null]`)); got != "" {
		t.Fatalf("cursorValue with a null part = %q, want empty", got)
	}
	if got := cursorValue(json.RawMessage(`"abc"`)); got != "abc" {
		t.Fatalf("cursorValue string = %q", got)
	}
}

func TestSaasRuleTriggerSyncFollowsArrayCursor(t *testing.T) {
	testenv.Isolate(t)
	var pages atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/respondApi/api/v1/auth":
			w.Write([]byte(`{"accessToken":"t"}`))
		case "/respondApi/api/v1/rule-triggers":
			pages.Add(1)
			after := r.URL.Query()["startAfter"]
			switch {
			case len(after) == 0:
				w.Write([]byte(`{"ruleTriggers":[{"id":"RT_1","status":"PENDING","triggeredAt":1686299747000},{"id":"RT_2","status":"PENDING","triggeredAt":1686299747517}],"pagination":{"hasNextPage":true,"cursor":[1686299747517,"RT_2"]}}`))
			case len(after) == 2 && after[0] == "1686299747517" && after[1] == "RT_2":
				w.Write([]byte(`{"ruleTriggers":[{"id":"RT_3","status":"APPROVED","triggeredAt":1686299748000}],"pagination":{"hasNextPage":false,"cursor":null}}`))
			default:
				w.WriteHeader(http.StatusBadRequest)
				w.Write([]byte(`bad startAfter: ` + strings.Join(after, "|")))
			}
		default:
			w.Write([]byte(`[]`))
		}
	}))
	defer srv.Close()
	t.Setenv("SAAS_ALERTS_BASE_URL", srv.URL)
	t.Setenv("SAAS_ALERTS_API_KEY", "test-partner-key")
	t.Setenv("PRINTING_PRESS_VERIFY", "")
	t.Setenv("PRINTING_PRESS_DOGFOOD", "")
	dbPath := filepath.Join(t.TempDir(), "data.db")

	cmd := RootCmd()
	var out, errb bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&errb)
	cmd.SetArgs([]string{"--no-learn", "--rate-limit", "0", "sync", "--resources", "rule-triggers", "--db", dbPath, "--json"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("sync: %v\nstdout: %s\nstderr: %s", err, out.String(), errb.String())
	}
	if pages.Load() != 2 {
		t.Fatalf("rule-trigger pages fetched = %d, want 2\nstderr: %s", pages.Load(), errb.String())
	}
	db, err := store.OpenReadOnlyContext(context.Background(), dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var n int
	if err := db.DB().QueryRow(`SELECT COUNT(*) FROM resources WHERE resource_type = 'rule-triggers'`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 3 {
		t.Fatalf("stored rule triggers = %d, want 3", n)
	}
}

// saasSyncRuleTriggers runs sync against a server that answers rule-triggers with
// the given page bodies in order, returning pages fetched and stderr.
func saasSyncRuleTriggers(t *testing.T, bodies ...string) (int32, string, int) {
	t.Helper()
	testenv.Isolate(t)
	return saasSyncRuleTriggersAt(t, filepath.Join(t.TempDir(), "data.db"), bodies...)
}

// saasSyncRuleTriggersAt runs one --full sync into dbPath (callers isolate env).
func saasSyncRuleTriggersAt(t *testing.T, dbPath string, bodies ...string) (int32, string, int) {
	t.Helper()
	var pages atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/respondApi/api/v1/auth" {
			w.Write([]byte(`{"accessToken":"t"}`))
			return
		}
		if r.URL.Path != "/respondApi/api/v1/rule-triggers" {
			w.Write([]byte(`[]`))
			return
		}
		i := int(pages.Add(1)) - 1
		if i >= len(bodies) {
			w.WriteHeader(http.StatusBadRequest)
			w.Write([]byte(`unexpected extra page`))
			return
		}
		w.Write([]byte(bodies[i]))
	}))
	defer srv.Close()
	t.Setenv("SAAS_ALERTS_BASE_URL", srv.URL)
	t.Setenv("SAAS_ALERTS_API_KEY", "test-partner-key")
	t.Setenv("PRINTING_PRESS_VERIFY", "")
	t.Setenv("PRINTING_PRESS_DOGFOOD", "")
	cmd := RootCmd()
	var out, errb bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&errb)
	// --full prunes, so the sync reports whether the walk was proven complete.
	cmd.SetArgs([]string{"--no-learn", "--rate-limit", "0", "sync", "--resources", "rule-triggers", "--db", dbPath, "--full", "--json"})
	_ = cmd.Execute()
	db, err := store.OpenReadOnlyContext(context.Background(), dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var n int
	_ = db.DB().QueryRow(`SELECT COUNT(*) FROM resources WHERE resource_type = 'rule-triggers'`).Scan(&n)
	return pages.Load(), errb.String(), n
}

func TestSaasRuleTriggerSyncHonorsHasNextPage(t *testing.T) {
	t.Run("false with a stale cursor stops", func(t *testing.T) {
		pages, stderr, n := saasSyncRuleTriggers(t,
			`{"ruleTriggers":[{"id":"RT_1","status":"PENDING","triggeredAt":1}],"pagination":{"hasNextPage":false,"cursor":[1,"RT_1"]}}`)
		if pages != 1 || n != 1 || !strings.Contains(stderr, `"event":"reconcile","resource":"rule-triggers"`) {
			t.Fatalf("want a proven-complete walk; pages=%d rows=%d stderr=%s", pages, n, stderr)
		}
	})
	t.Run("true without a cursor is not complete", func(t *testing.T) {
		pages, stderr, n := saasSyncRuleTriggers(t,
			`{"ruleTriggers":[{"id":"RT_1","status":"PENDING","triggeredAt":1}],"pagination":{"hasNextPage":true,"cursor":null}}`)
		if pages != 1 || n != 1 || !strings.Contains(stderr, `"reason":"cursor_unavailable"`) || strings.Contains(stderr, `"event":"reconcile","resource":"rule-triggers"`) {
			t.Fatalf("want an incomplete-sync signal; pages=%d rows=%d stderr=%s", pages, n, stderr)
		}
	})
}

func saasTriggerPage(start, n int, pagination string) string {
	rows := make([]string, 0, n)
	for i := start; i < start+n; i++ {
		rows = append(rows, `{"id":"RT_`+strconv.Itoa(i)+`","status":"PENDING","triggeredAt":`+strconv.Itoa(1000+i)+`}`)
	}
	return `{"ruleTriggers":[` + strings.Join(rows, ",") + `],"pagination":` + pagination + `}`
}

func TestSaasRuleTriggerSyncFullTerminalPageIsComplete(t *testing.T) {
	for name, pagination := range map[string]string{
		"null cursor":     `{"hasNextPage":false,"cursor":null}`,
		"retained cursor": `{"hasNextPage":false,"cursor":[1099,"RT_99"]}`,
	} {
		t.Run(name, func(t *testing.T) {
			pages, stderr, n := saasSyncRuleTriggers(t, saasTriggerPage(0, 100, pagination))
			if pages != 1 || n != 100 || !strings.Contains(stderr, `"event":"reconcile","resource":"rule-triggers"`) {
				t.Fatalf("full terminal page must be a proven end; pages=%d rows=%d stderr=%s", pages, n, stderr)
			}
		})
	}
	t.Run("three-element cursor is unusable", func(t *testing.T) {
		pages, stderr, _ := saasSyncRuleTriggers(t, saasTriggerPage(0, 100, `{"hasNextPage":true,"cursor":[1,"RT_99","extra"]}`))
		if pages != 1 || !strings.Contains(stderr, `"reason":"cursor_unavailable"`) {
			t.Fatalf("want cursor_unavailable; pages=%d stderr=%s", pages, stderr)
		}
	})
}

func TestSaasRuleTriggerSyncNullFlagNeverPrunes(t *testing.T) {
	for name, pagination := range map[string]string{
		"null flag, usable cursor": `{"hasNextPage":null,"cursor":[1099,"RT_99"]}`,
		"null flag, no cursor":     `{"hasNextPage":null,"cursor":null}`,
		"string flag":              `{"hasNextPage":"no","cursor":null}`,
	} {
		t.Run(name, func(t *testing.T) {
			_, stderr, _ := saasSyncRuleTriggers(t, saasTriggerPage(0, 100, pagination))
			if strings.Contains(stderr, `"event":"reconcile","resource":"rule-triggers"`) {
				t.Fatalf("a non-boolean continuation flag must never prove the end (prune ran): %s", stderr)
			}
		})
	}
}

func TestSaasRuleTriggerUnprovenEndNeverPrunes(t *testing.T) {
	for name, page := range map[string]string{
		"empty page, null flag":       `{"ruleTriggers":[],"pagination":{"hasNextPage":null,"cursor":null}}`,
		"empty page, true, no cursor": `{"ruleTriggers":[],"pagination":{"hasNextPage":true,"cursor":null}}`,
		"short page, no pagination":   `{"ruleTriggers":[{"id":"RT_1","status":"PENDING","triggeredAt":1}]}`,
	} {
		t.Run(name, func(t *testing.T) {
			testenv.Isolate(t)
			dbPath := filepath.Join(t.TempDir(), "data.db")
			if _, stderr, n := saasSyncRuleTriggersAt(t, dbPath, saasTriggerPage(0, 3, `{"hasNextPage":false,"cursor":null}`)); n != 3 {
				t.Fatalf("seed sync stored %d rows: %s", n, stderr)
			}
			_, stderr, n := saasSyncRuleTriggersAt(t, dbPath, page)
			if strings.Contains(stderr, `"event":"reconcile","resource":"rule-triggers"`) || n < 3 {
				t.Fatalf("unproven end pruned stored triggers: rows=%d stderr=%s", n, stderr)
			}
			if !strings.Contains(stderr, "pagination_flag_missing") {
				t.Fatalf("want pagination_flag_missing: %s", stderr)
			}
		})
	}
}

func TestSaasCollectionErrorBodyNeverPrunes(t *testing.T) {
	testenv.Isolate(t)
	dbPath := filepath.Join(t.TempDir(), "data.db")
	if _, stderr, n := saasSyncRuleTriggersAt(t, dbPath, saasTriggerPage(0, 3, `{"hasNextPage":false,"cursor":null}`)); n != 3 {
		t.Fatalf("seed sync stored %d rows: %s", n, stderr)
	}
	for _, body := range []string{
		`{"id":"REQUEST_ERROR_123","message":"backend failure"}`,
		`{"errors":[],"message":"backend failure","pagination":{"hasNextPage":false}}`,
		`{"ruleTriggers":null,"total":0,"pagination":{"hasNextPage":false}}`,
		`[]`,
		`{"ruleTriggers":[],"error":"backend failure","pagination":{"hasNextPage":false}}`,
		`{"ruleTriggers":[],"errors":["backend failure"],"pagination":{"hasNextPage":false}}`,
		`{"ruleTriggers":[],"success":false,"pagination":{"hasNextPage":false}}`,
		`{"ruleTriggers":[],"message":"backend failure","pagination":{"hasNextPage":false}}`,
		`{"ruleTriggers":[],"success":true,"status":"ERROR","pagination":{"hasNextPage":false}}`,
		`{"ruleTriggers":[],"ok":false,"pagination":{"hasNextPage":false}}`,
		`{"ruleTriggers":[],"ok":true,"error":"x","pagination":{"hasNextPage":false}}`,
		`{"ruleTriggers":[],"code":500,"pagination":{"hasNextPage":false}}`,
		`{"ruleTriggers":[{"id":"REQUEST_ERROR_123","status":"PENDING","success":false,"error":"backend failure"}],"pagination":{"hasNextPage":false}}`,
		`{"ruleTriggers":[{"id":"REQUEST_ERROR_123","status":"PENDING","ok":false}],"pagination":{"hasNextPage":false}}`,
		`{"ruleTriggers":[{"id":"REQUEST_ERROR_123","status":"PENDING","code":500}],"pagination":{"hasNextPage":false}}`,
		`{"ruleTriggers":[{"id":"RT_OK","status":"PENDING","triggeredAt":1},{"status":"PENDING","triggeredAt":2}],"pagination":{"hasNextPage":false}}`,
		`{"ruleTriggers":[{"id":"RT_OK","status":"PENDING","triggeredAt":1},null],"pagination":{"hasNextPage":false}}`,
		`{"ruleTriggers":["REQUEST_ERROR_123"],"pagination":{"hasNextPage":false}}`,
		`{"ruleTriggers":[{"id":"REQUEST_ERROR_123","status":"ERROR"}],"pagination":{"hasNextPage":false}}`,
		`{"ruleTriggers":[{"id":"REQUEST_ERROR_123"}],"pagination":{"hasNextPage":false}}`,
		`{"ruleTriggers":[{"id":"REQUEST_ERROR_123","status":null}],"pagination":{"hasNextPage":false}}`,
		`{"ruleTriggers":[{"id":"REQUEST_ERROR_123","status":500}],"pagination":{"hasNextPage":false}}`,
		`{"ruleTriggers":[{"id":"REQUEST_ERROR_123","status":"pending"}],"pagination":{"hasNextPage":false}}`,
	} {
		_, stderr, n := saasSyncRuleTriggersAt(t, dbPath, body)
		if n != 3 || strings.Contains(stderr, `"event":"reconcile","resource":"rule-triggers"`) {
			t.Fatalf("%s pruned or replaced stored triggers: rows=%d stderr=%s", body, n, stderr)
		}
		if !strings.Contains(stderr, "not the documented collection response") {
			t.Fatalf("%s: want an envelope anomaly: %s", body, stderr)
		}
	}
	db, err := store.OpenReadOnlyContext(context.Background(), dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var bogus int
	_ = db.DB().QueryRow(`SELECT COUNT(*) FROM resources WHERE id IN ('REQUEST_ERROR_123', 'RT_OK')`).Scan(&bogus)
	if bogus != 0 {
		t.Fatal("the error body was stored as a trigger")
	}
}

func saasSyncEventsAt(t *testing.T, dbPath, body string) (string, int) {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/reportApi/api/v1/reports/events" {
			w.Write([]byte(body))
			return
		}
		w.Write([]byte(`[]`))
	}))
	defer srv.Close()
	t.Setenv("SAAS_ALERTS_BASE_URL", srv.URL)
	t.Setenv("SAAS_ALERTS_API_KEY", "test-partner-key")
	t.Setenv("PRINTING_PRESS_VERIFY", "")
	t.Setenv("PRINTING_PRESS_DOGFOOD", "")
	cmd := RootCmd()
	var out, errb bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&errb)
	cmd.SetArgs([]string{"--no-learn", "--rate-limit", "0", "sync", "--resources", "events", "--db", dbPath, "--full", "--json"})
	_ = cmd.Execute()
	db, err := store.OpenReadOnlyContext(context.Background(), dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var n int
	_ = db.DB().QueryRow(`SELECT COUNT(*) FROM resources WHERE resource_type = 'events'`).Scan(&n)
	return errb.String(), n
}

func TestSaasEventsWrapperErrorNeverPrunes(t *testing.T) {
	testenv.Isolate(t)
	dbPath := filepath.Join(t.TempDir(), "data.db")
	seed := `[{"eventId":"logins_1","time":"2026-10-13T07:36:46Z","customer":{"id":"C1"},"product":{"name":"Microsoft 365"},"user":{"id":"U1"}},` +
		`{"eventId":"logins_2","time":"2026-10-13T07:36:47Z","customer":{"id":"C1"},"product":{"name":"Microsoft 365"},"user":{"id":"U2"}}]`
	if stderr, n := saasSyncEventsAt(t, dbPath, seed); n != 2 {
		t.Fatalf("seed sync stored %d events: %s", n, stderr)
	}
	for _, body := range []string{`{"items":[],"error":"backend failure"}`, `{"data":[]}`, `{"message":"backend failure"}`,
		`[{"_id":"doc-9","time":"2026-10-14T12:00:00Z","success":false,"error":"backend failure"}]`,
		`[{"eventId":"logins_3","time":"2026-10-14T12:00:00Z","ok":false}]`,
		// a seeded event again (same identity, so the row count is unchanged) beside a scalar member
		`[{"eventId":"logins_1","time":"2026-10-13T07:36:46Z","customer":{"id":"C1"},"product":{"name":"Microsoft 365"},"user":{"id":"U1"}},"backend failure"]`} {
		stderr, n := saasSyncEventsAt(t, dbPath, body)
		if n != 2 || strings.Contains(stderr, `"event":"reconcile","resource":"events"`) {
			t.Fatalf("%s pruned or replaced stored events: rows=%d stderr=%s", body, n, stderr)
		}
		// A malformed envelope is a collection anomaly; a failure-bearing member is
		// rejected at storage, which fails the resource. Either way the sync errors.
		if !strings.Contains(stderr, "not the documented collection response") && !strings.Contains(stderr, `"event":"sync_error","resource":"events"`) {
			t.Fatalf("%s: want a collection anomaly or a sync error: %s", body, stderr)
		}
	}
}
