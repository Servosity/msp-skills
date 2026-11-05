// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"superops-pp-cli/internal/cliutil/testenv"
	"superops-pp-cli/internal/credkit"
)

const reviewToken = "sop-test-token-4471b9"

// reviewTokenForms is the credential as written and in the encodings a vendor
// error message could echo it in.
func reviewTokenForms() []string {
	return []string{
		reviewToken,
		base64.StdEncoding.EncodeToString([]byte(reviewToken)),
		base64.RawURLEncoding.EncodeToString([]byte(reviewToken)),
		hex.EncodeToString([]byte(reviewToken)),
	}
}

// captureProcessStderr runs fn with os.Stderr redirected: sync writes its
// events straight to the process stderr, not to the command's writer.
func captureProcessStderr(t *testing.T, fn func()) string {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	orig := os.Stderr
	os.Stderr = w
	done := make(chan string, 1)
	go func() {
		b, _ := io.ReadAll(r)
		done <- string(b)
	}()
	defer func() { os.Stderr = orig }()
	fn()
	os.Stderr = orig
	_ = w.Close()
	return <-done
}

func graphQLErrorBody(t *testing.T, message, code string) string {
	t.Helper()
	e := map[string]any{"message": message}
	if code != "" {
		e["extensions"] = map[string]any{"code": code}
	}
	b, err := json.Marshal(map[string]any{"errors": []any{e}})
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func serveGraphQLBody(t *testing.T, body string) {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, body)
	}))
	t.Cleanup(srv.Close)
	testenv.Isolate(t)
	t.Setenv("SUPEROPS_BASE_URL", srv.URL)
	t.Setenv("SUPEROPS_API_TOKEN", reviewToken)
	t.Setenv("SUPEROPS_SUBDOMAIN", "acme-msp")
}

// A GraphQL error arrives with HTTP 200, so the HTTP error path never masked
// it, and sync prints its events to stderr before any command-error funnel
// runs. The credential must not appear in any form, in any output mode, for
// either error shape (access denial -> sync_warning, other -> sync_error).
func TestSyncNeverPrintsAnEchoedCredential(t *testing.T) {
	for _, form := range reviewTokenForms() {
		for _, code := range []string{"FORBIDDEN", "UNAUTHENTICATED", "", "INTERNAL_SERVER_ERROR"} {
			for _, mode := range [][]string{nil, {"--human-friendly"}} {
				name := fmt.Sprintf("%s/%s/%v", form[:6], code, mode)
				t.Run(name, func(t *testing.T) {
					serveGraphQLBody(t, graphQLErrorBody(t, "token "+form+" is not valid for Bearer "+form, code))
					var out string
					stderr := captureProcessStderr(t, func() {
						args := append([]string{"sync", "--resources", "tickets"}, mode...)
						out, _ = runSuperOpsCLI(t, args...)
					})
					for _, f := range reviewTokenForms() {
						if strings.Contains(stderr, f) || strings.Contains(out, f) {
							t.Fatalf("sync printed the credential (%s):\nstderr:\n%s\nout:\n%s", f, stderr, out)
						}
					}
				})
			}
		}
	}
}

// doctor prints only GraphQL error codes, but a code is vendor text too: one
// that repeats the credential is not shown.
func TestDoctorNeverPrintsACredentialShapedErrorCode(t *testing.T) {
	if !heldChild(t) {
		return
	}
	credkit.Hold("minted:test", reviewToken)
	for _, form := range []string{reviewToken, base64.RawURLEncoding.EncodeToString([]byte(reviewToken))} {
		t.Run(form[:6], func(t *testing.T) {
			serveGraphQLBody(t, graphQLErrorBody(t, "denied", form))
			out, _ := runSuperOpsCLI(t, "doctor", "--json")
			for _, f := range reviewTokenForms() {
				if strings.Contains(out, f) {
					t.Fatalf("doctor printed the credential (%s):\n%s", f, out)
				}
			}
		})
	}
}

// tenantFakes routes each request to one tenant's fake by the
// CustomerSubDomain header, the way SuperOps scopes every query.
type tenantFakes map[string]*fakeSuperOps

func (ft tenantFakes) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f, ok := ft[r.Header.Get("CustomerSubDomain")]
	if !ok {
		http.Error(w, "unknown tenant", http.StatusBadRequest)
		return
	}
	f.ServeHTTP(w, r)
}

// The SuperOps tenant is the CustomerSubDomain header, not part of the base
// URL, so neither the store scope nor the sync checkpoint scope saw it: tenant
// B resumed tenant A's page cursor and B's local reads included A's rows.
func TestSyncKeepsEachTenantApart(t *testing.T) {
	for _, shared := range []bool{false, true} {
		t.Run(fmt.Sprintf("shared-db=%v", shared), func(t *testing.T) {
			now := time.Now()
			bTickets := fakeTickets(10, now)
			for i := range bTickets {
				bTickets[i]["ticketId"] = fmt.Sprintf("80000000000000%04d", i+1)
				bTickets[i]["displayId"] = fmt.Sprintf("B-%d", i+1)
			}
			a := &fakeSuperOps{t: t, data: map[string][]map[string]any{"getTicketList": fakeTickets(120, now)}}
			b := &fakeSuperOps{t: t, data: map[string][]map[string]any{"getTicketList": bTickets}}
			srv := httptest.NewServer(tenantFakes{"acme-a": a, "acme-b": b})
			t.Cleanup(srv.Close)
			testenv.Isolate(t)
			t.Setenv("SUPEROPS_BASE_URL", srv.URL)
			t.Setenv("SUPEROPS_API_TOKEN", reviewToken)
			var extra []string
			if shared {
				extra = []string{"--db", filepath.Join(t.TempDir(), "shared.db")}
			}

			t.Setenv("SUPEROPS_SUBDOMAIN", "acme-a")
			_, _ = runSuperOpsCLI(t, append([]string{"sync", "--resources", "tickets", "--max-pages", "1"}, extra...)...)
			if got := fmt.Sprint(a.pages["getTicketList"]); got != "[1]" {
				t.Fatalf("tenant A pages %s, want [1]", got)
			}

			t.Setenv("SUPEROPS_SUBDOMAIN", "acme-b")
			out, err := runSuperOpsCLI(t, append([]string{"sync", "--resources", "tickets"}, extra...)...)
			if shared {
				// A --db shared by two tenants is tenant A's once A filled it:
				// B is refused before any request (superops-tenant-scope stamp).
				if err == nil || !strings.Contains(out, "different SuperOps tenant") {
					t.Fatalf("tenant B sync into tenant A's --db was not refused (err %v):\n%s", err, out)
				}
				if got := len(b.pages["getTicketList"]); got != 0 {
					t.Fatalf("tenant B fetched %d pages into tenant A's --db", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("tenant B sync: %v\n%s", err, out)
			}
			if got := fmt.Sprint(b.pages["getTicketList"]); got != "[1]" {
				t.Fatalf("tenant B pages %s, want [1]: it resumed tenant A's checkpoint", got)
			}
			if got := superopsStoreRows(t, "tickets"); got != 10 {
				t.Fatalf("tenant B's store holds %d tickets, want its own 10", got)
			}
			out, err = runSuperOpsCLI(t, "stale-tickets", "--days", "7", "--json")
			if err != nil {
				t.Fatalf("stale-tickets: %v\n%s", err, out)
			}
			if strings.Contains(out, `"T-1"`) || !strings.Contains(out, `"B-1"`) {
				t.Fatalf("tenant B's local read is not tenant B's data:\n%s", out)
			}
		})
	}
}

// A local novel under --dry-run says what it would do instead of exiting 0
// with empty output.
func TestLocalNovelsPreviewUnderDryRun(t *testing.T) {
	testenv.Isolate(t)
	for _, args := range [][]string{
		{"sla-watch"}, {"unbilled"}, {"at-risk-assets"}, {"alert-coverage"}, {"stale-tickets"},
		{"client-360", "Acme"}, {"context-ticket", "T-1"},
	} {
		out, err := runSuperOpsCLI(t, append(args, "--dry-run", "--json")...)
		if err != nil {
			t.Fatalf("%v --dry-run: %v\n%s", args, err, out)
		}
		var got struct {
			DryRun bool   `json:"dry_run"`
			Action string `json:"action"`
		}
		if err := json.Unmarshal([]byte(strings.TrimSpace(out)), &got); err != nil || !got.DryRun || got.Action != args[0] {
			t.Fatalf("%v --dry-run --json printed %q, want a dry_run result for %s", args, out, args[0])
		}
	}
}
