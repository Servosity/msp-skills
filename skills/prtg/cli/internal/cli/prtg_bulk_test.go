// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"prtg-pp-cli/internal/cliutil/testenv"
)

// documentedFailure is PRTG's "200: Success with one failure" ActionResult
// example (prtg.api.yaml, /sensors/resume).
const documentedFailure = `[{"node":{"id":"2322","type":"REFERENCED_SENSOR","name":"Ping","href":"/api/v2/sensors/2322"},"failure":{"code":"BAD_REQUEST","message":"The request could not be processed. Check if the format and the data types are correct.","request_id":"amAUOX"}},{"node":{"id":"2323","type":"REFERENCED_SENSOR","name":"HTTP","href":"/api/v2/sensors/2323"},"failure":null}]`

// bulk-action-outcomes: a generated bulk action reports success only when
// every item succeeded; one failed item fails the command with per-item detail.
func TestBulkActionPerItemFailureFailsTheCommand(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(documentedFailure))
	}))
	t.Cleanup(srv.Close)
	testenv.Isolate(t)
	t.Setenv("PRTG_SERVER", strings.TrimPrefix(srv.URL, "https://"))
	t.Setenv("PRTG_API_KEY", "bulkTestKey-0123456789")
	t.Setenv("PRTG_SKIP_TLS_VERIFY", "true")
	t.Cleanup(func() { insecureTLS = false })
	out, _, err := runRootArgs(t, "sensors", "multi-resume", "--filter", `name = "Ping"`, "--json")
	if err == nil {
		t.Fatalf("a bulk action with a failed item exited 0:\n%s", out)
	}
	var env map[string]any
	if json.Unmarshal([]byte(out), &env) != nil || env["success"] != false {
		t.Fatalf("want success false, got:\n%s", out)
	}
	if !strings.Contains(out, "2322") || !strings.Contains(out, "BAD_REQUEST") {
		t.Fatalf("want per-item failure detail, got:\n%s", out)
	}
}

// The fleet action engine reads the same results: any failure signal in an
// item, a populated failure object whatever it says included, fails it.
func TestFleetActionResultsAnyFailureSignalWins(t *testing.T) {
	for name, body := range map[string]string{
		"documented":        documentedFailure,
		"empty failure":     `[{"node":{"id":"1","name":"a"},"failure":{}}]`,
		"error field":       `[{"node":{"id":"1","name":"a"},"error":"denied"}]`,
		"status wording":    `[{"node":{"id":"1","name":"a"},"status":"FAILED"}]`,
		"http status":       `[{"node":{"id":"1","name":"a"},"status":500}]`,
		"success false":     `[{"node":{"id":"1","name":"a"},"success":false}]`,
		"failure code only": `[{"node":{"id":"1","name":"a"},"failure":{"code":"FORBIDDEN"}}]`,
		"failure as string": `[{"node":{"id":"1","name":"a"},"failure":"no access"}]`,
		"failure, no node":  `[{"failure":{"message":"x"}}]`,
	} {
		res, err := actionResults(json.RawMessage(body))
		if err == nil && res.failed == 0 {
			t.Errorf("%s: read as a success", name)
		}
	}
	res, err := actionResults(json.RawMessage(`[{"node":{"id":"1","name":"a"},"failure":null},{"node":{"id":"2","name":"b"}}]`))
	if err != nil || res.failed != 0 || len(res.okIDs) != 2 {
		t.Fatalf("clean results misread: %+v %v", res, err)
	}
}
