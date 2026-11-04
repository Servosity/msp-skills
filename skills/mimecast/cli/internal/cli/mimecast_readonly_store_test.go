// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

// Hand-written (NOT generated; handfixes.json mimecast-read-only-workflows-write-nothing).

package cli

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"mimecast-pp-cli/internal/cliutil/testenv"
)

// A command the MCP server exposes as read-only runs without confirm, so under
// the MCP server, and with MIMECAST_NO_LOCAL_DATA=1, it must write nothing:
// no workflow database is created and no release log is cached. The answer
// still comes from the live API.
func TestReadOnlyWorkflowsWriteNothing(t *testing.T) {
	for _, env := range []string{"MIMECAST_MCP_CHILD", "MIMECAST_NO_LOCAL_DATA"} {
		t.Run(env, func(t *testing.T) {
			testenv.Isolate(t)
			t.Setenv(env, "1")
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				switch r.URL.Path {
				case "/oauth/token":
					_, _ = io.WriteString(w, `{"access_token":"tok-123","token_type":"Bearer","expires_in":1800}`)
				case "/api/gateway/get-held-release-logs":
					rows := []string{}
					for i := 0; i < 3; i++ {
						rows = append(rows, `{"id":"r`+string(rune('a'+i))+`","status":"released","released":"`+mcTime(time.Now().Add(-time.Duration(i+1)*time.Hour))+`","fromEnv":{"emailAddress":"news@vendor.example"},"to":[{"emailAddress":"alice@example.com"}],"operator":{"emailAddress":"admin@example.com"},"heldReason":"Spam","subject":"Weekly news"}`)
					}
					_, _ = io.WriteString(w, `{"meta":{"status":200},"data":[{"heldReleaseLogs":[`+strings.Join(rows, ",")+`]}],"fail":[]}`)
				default:
					w.WriteHeader(http.StatusNotFound)
				}
			}))
			defer srv.Close()

			out, err := runMimecast(t, srv, "held", "release-audit", "--since", "7d")
			var av releaseAuditView
			if err != nil || json.Unmarshal([]byte(out), &av) != nil || av.ScannedLogs != 3 {
				t.Fatalf("release-audit: err=%v %s", err, out)
			}
			out, err = runMimecast(t, srv, "senders", "suggest", "--since", "7d", "--min-holds", "3")
			var sv sendersSuggestView
			if err != nil || json.Unmarshal([]byte(out), &sv) != nil || len(sv.PermitCandidates) != 1 || sv.PermitCandidates[0].Sender != "news@vendor.example" {
				t.Fatalf("senders suggest must still answer from the live window: err=%v %s", err, out)
			}
			if _, err := os.Stat(defaultDBPath("mimecast-cli")); !os.IsNotExist(err) {
				t.Fatalf("a read-only workflow wrote the local store under %s=1 (stat err=%v)", env, err)
			}
		})
	}
}

// local-data purge finds every workflow store, the per-account ones included.
func TestPurgeFindsAccountStores(t *testing.T) {
	for _, name := range []string{"data.db", "data-ab12.db-wal", "data-ab12-account-ACME_1.db", "data-account-x-y.db.owner", "data-ab12-account-ACME.db-shm"} {
		if !localStoreFilePattern.MatchString(name) {
			t.Errorf("purge misses %s", name)
		}
	}
	if localStoreFilePattern.MatchString("data-ab12-account-.db") {
		t.Error("an empty account code is not a store name")
	}
}
