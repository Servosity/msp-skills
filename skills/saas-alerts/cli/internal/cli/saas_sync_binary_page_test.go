// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
//
// Hand-written (NOT generated): pinned by skills/saas-alerts/handfixes.json
// under the "sync-binary-page-not-an-enumeration" entry.

package cli

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"saas-alerts-pp-cli/internal/cliutil/testenv"
)

// TestSyncBinaryLabelledPageNeverPrunes: a page whose answer is labelled binary
// (outside the Webhooks API, which is always read as JSON) is not an enumeration, so a full sync stores nothing from it and prunes
// nothing that was already synced.
func TestSyncBinaryLabelledPageNeverPrunes(t *testing.T) {
	var mu sync.Mutex
	ctype := "application/json"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		w.Header().Set("Content-Type", ctype)
		_, _ = w.Write([]byte(`[{"id":"CUST_1","name":"Contoso"},{"id":"CUST_2","name":"Fabrikam"}]`))
	}))
	defer srv.Close()
	testenv.Isolate(t)
	t.Setenv("SAAS_ALERTS_BASE_URL", srv.URL)
	t.Setenv("SAAS_ALERTS_API_KEY", "test-key-0000")
	t.Setenv("PRINTING_PRESS_VERIFY", "")
	t.Setenv("PRINTING_PRESS_DOGFOOD", "")

	if out, _, err := rqtRun(t, "sync", "--resources", "customers", "--full", "--json"); err != nil {
		t.Fatalf("json sync: %v\n%s", err, out)
	}
	mu.Lock()
	ctype = "application/octet-stream"
	mu.Unlock()
	out, stderr, _ := rqtRun(t, "sync", "--resources", "customers", "--full", "--json")
	if !strings.Contains(out+stderr, "non_json_200_body") {
		t.Fatalf("a binary-labelled page was not flagged:\n%s\n%s", out, stderr)
	}
	list, _, err := rqtRun(t, "customers", "list", "--data-source", "local", "--json")
	if err != nil || !strings.Contains(list, "CUST_1") || !strings.Contains(list, "CUST_2") {
		t.Fatalf("synced rows were pruned: err=%v\n%s", err, list)
	}
}
