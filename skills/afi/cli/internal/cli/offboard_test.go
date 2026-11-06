// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
package cli

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"afi-pp-cli/internal/client"
)

// offboardServer fakes the four Afi endpoints the offboard sequence touches.
type offboardServer struct {
	mu          sync.Mutex
	protections string // JSON page body
	taskStatus  string
	archiveAt   string // created_at for the newest archive; empty = no archives
	unprotected bool
	triggered   bool
}

func (s *offboardServer) handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/tenants/ten1/protections", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, s.protections)
	})
	mux.HandleFunc("/api/v1/tenants/ten1/jobs/job1/trigger", func(w http.ResponseWriter, r *http.Request) {
		s.mu.Lock()
		s.triggered = true
		s.mu.Unlock()
		fmt.Fprint(w, `{"task_id":"task1"}`)
	})
	mux.HandleFunc("/api/v1/tenants/ten1/tasks/task1", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, `{"id":"task1","status":%q}`, s.taskStatus)
	})
	mux.HandleFunc("/api/v1/tenants/ten1/archives", func(w http.ResponseWriter, r *http.Request) {
		if s.archiveAt == "" {
			fmt.Fprint(w, `{"items":[],"next_page_token":""}`)
			return
		}
		body, _ := json.Marshal(map[string]any{
			"items": []map[string]any{{"id": "arc1", "resource_id": "res1", "created_at": s.archiveAt}},
		})
		w.Write(body)
	})
	mux.HandleFunc("/api/v1/tenants/ten1/resources/res1/protect", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodDelete {
			s.mu.Lock()
			s.unprotected = true
			s.mu.Unlock()
		}
		fmt.Fprint(w, `{}`)
	})
	return mux
}

func runOffboard(t *testing.T, srvURL string, args ...string) (string, error) {
	t.Helper()
	// In-process tests run no credkit.Start, so the kit's transport (the one
	// client hook, credkit_client.go) would refuse the key to this loopback
	// fake. Offboard's own sequence is under test here; the kit's routing is
	// proven on the built binaries (internal/credwire). The response cache stays
	// off, as kitClient leaves it.
	saved := clientHooks
	clientHooks = []func(*client.Client) error{func(c *client.Client) error { c.NoCache = true; return nil }}
	t.Cleanup(func() { clientHooks = saved })
	t.Setenv("AFI_BASE_URL", srvURL)
	t.Setenv("AFI_API_KEY", "appkey-test")
	t.Setenv("AFI_CONFIG", filepath.Join(t.TempDir(), "config.toml"))
	t.Setenv("PRINTING_PRESS_VERIFY", "")
	t.Setenv("PRINTING_PRESS_DOGFOOD", "")
	return runNovel(t, newNovelOffboardCmd, args...)
}

func TestOffboardHappyPath(t *testing.T) {
	srv := &offboardServer{
		protections: `{"items":[{"id":"prot1","resource_id":"res1","policy_id":"pol1","job_id":"job1"}],"next_page_token":""}`,
		taskStatus:  "done",
		archiveAt:   time.Now().UTC().Format(time.RFC3339),
	}
	ts := httptest.NewServer(srv.handler())
	defer ts.Close()

	out, err := runOffboard(t, ts.URL, "res1", "--tenant", "ten1", "--poll-interval", "10ms", "--reason", "test departure")
	if err != nil {
		t.Fatalf("offboard: %v\n%s", err, out)
	}
	view := decodeView[offboardView](t, out)
	if !view.Unprotected || view.ArchiveID != "arc1" || view.TaskID != "task1" || view.PolicyID != "pol1" {
		t.Errorf("receipt wrong: %s", out)
	}
	if !srv.triggered || !srv.unprotected {
		t.Errorf("server state: triggered=%v unprotected=%v, want both true", srv.triggered, srv.unprotected)
	}
	for _, s := range view.Steps {
		if s.Status == "failed" {
			t.Errorf("step %s failed: %s", s.Step, s.Detail)
		}
	}
}

func TestOffboardRefusesWithoutArchive(t *testing.T) {
	srv := &offboardServer{
		protections: `{"items":[{"id":"prot1","resource_id":"res1","policy_id":"pol1","job_id":"job1"}],"next_page_token":""}`,
		taskStatus:  "done",
		archiveAt:   "", // no archives at all
	}
	ts := httptest.NewServer(srv.handler())
	defer ts.Close()

	out, err := runOffboard(t, ts.URL, "res1", "--tenant", "ten1", "--poll-interval", "10ms")
	if err == nil || !strings.Contains(err.Error(), "REFUSING") {
		t.Fatalf("expected REFUSING error, got %v\n%s", err, out)
	}
	if srv.unprotected {
		t.Fatal("unprotect must NOT be called when no archive exists")
	}
}

// Codex r1 (afi review) class archive-creation-read-as-backup-time: a
// backup is appended to the resource's existing archive (spec.json v1Archive
// is a backup sequence), so a finished final-backup task is the proof, not a
// newly created archive.
func TestOffboardAcceptsABackupAppendedToAnExistingArchive(t *testing.T) {
	srv := &offboardServer{
		protections: `{"items":[{"id":"prot1","resource_id":"res1","policy_id":"pol1","job_id":"job1"}],"next_page_token":""}`,
		taskStatus:  "done",
		archiveAt:   time.Now().UTC().Add(-24 * time.Hour).Format(time.RFC3339), // the sequence the backup extended
	}
	ts := httptest.NewServer(srv.handler())
	defer ts.Close()

	out, err := runOffboard(t, ts.URL, "res1", "--tenant", "ten1", "--poll-interval", "10ms")
	if err != nil {
		t.Fatalf("a finished final backup into the existing archive was refused: %v\n%s", err, out)
	}
	if !srv.unprotected {
		t.Fatal("unprotect was not called after the final backup task finished")
	}
}

// A task that ends with warnings (a documented terminal status) is not a
// verified backup: refused at once, not after the wait budget.
func TestOffboardWarningsTaskBlocks(t *testing.T) {
	srv := &offboardServer{
		protections: `{"items":[{"id":"prot1","resource_id":"res1","policy_id":"pol1","job_id":"job1"}],"next_page_token":""}`,
		taskStatus:  "warnings",
		archiveAt:   time.Now().UTC().Format(time.RFC3339),
	}
	ts := httptest.NewServer(srv.handler())
	defer ts.Close()

	_, err := runOffboard(t, ts.URL, "res1", "--tenant", "ten1", "--poll-interval", "10ms", "--timeout-wait", "2s")
	if err == nil || !strings.Contains(err.Error(), "completed with warnings") {
		t.Fatalf("expected a refusal naming the warnings, got %v", err)
	}
	if srv.unprotected {
		t.Fatal("unprotect must NOT be called after a backup that completed with warnings")
	}
}

func TestOffboardNoWaitAcceptsExistingArchive(t *testing.T) {
	srv := &offboardServer{
		protections: `{"items":[{"id":"prot1","resource_id":"res1","policy_id":"pol1","job_id":"job1"}],"next_page_token":""}`,
		taskStatus:  "running",
		archiveAt:   time.Now().UTC().Add(-24 * time.Hour).Format(time.RFC3339),
	}
	ts := httptest.NewServer(srv.handler())
	defer ts.Close()

	out, err := runOffboard(t, ts.URL, "res1", "--tenant", "ten1", "--no-wait")
	if err != nil {
		t.Fatalf("offboard --no-wait: %v\n%s", err, out)
	}
	if !srv.unprotected {
		t.Fatal("--no-wait with an existing archive should unprotect")
	}
}

func TestOffboardFailedTaskBlocks(t *testing.T) {
	srv := &offboardServer{
		protections: `{"items":[{"id":"prot1","resource_id":"res1","policy_id":"pol1","job_id":"job1"}],"next_page_token":""}`,
		taskStatus:  "failed",
		archiveAt:   time.Now().UTC().Format(time.RFC3339),
	}
	ts := httptest.NewServer(srv.handler())
	defer ts.Close()

	_, err := runOffboard(t, ts.URL, "res1", "--tenant", "ten1", "--poll-interval", "10ms")
	if err == nil || !strings.Contains(err.Error(), "NOT unprotecting") {
		t.Fatalf("expected failed-task block, got %v", err)
	}
	if srv.unprotected {
		t.Fatal("unprotect must NOT be called after a failed backup task")
	}
}

func TestOffboardMultipleProtectionsNeedPolicy(t *testing.T) {
	srv := &offboardServer{
		protections: `{"items":[{"id":"prot1","resource_id":"res1","policy_id":"pol1","job_id":"job1"},{"id":"prot2","resource_id":"res1","policy_id":"pol2","job_id":"job2"}],"next_page_token":""}`,
		taskStatus:  "done",
		archiveAt:   time.Now().UTC().Format(time.RFC3339),
	}
	ts := httptest.NewServer(srv.handler())
	defer ts.Close()

	_, err := runOffboard(t, ts.URL, "res1", "--tenant", "ten1")
	if err == nil || !strings.Contains(err.Error(), "--policy") {
		t.Fatalf("expected disambiguation error, got %v", err)
	}
}

func TestOffboardNoProtection(t *testing.T) {
	srv := &offboardServer{
		protections: `{"items":[],"next_page_token":""}`,
	}
	ts := httptest.NewServer(srv.handler())
	defer ts.Close()

	_, err := runOffboard(t, ts.URL, "res1", "--tenant", "ten1")
	if err == nil || !strings.Contains(err.Error(), "no protection") {
		t.Fatalf("expected no-protection error, got %v", err)
	}
}

func TestOffboardNoTaskIDRefuses(t *testing.T) {
	// The trigger returned no task_id (spec.json v1TriggerJobResult always
	// carries one): nothing can confirm the final backup finished, and a new
	// archive is no proof either (backups extend the existing one).
	srv := &offboardServer{
		protections: `{"items":[{"id":"prot1","resource_id":"res1","policy_id":"pol1","job_id":"job1"}],"next_page_token":""}`,
		taskStatus:  "done",
		archiveAt:   time.Now().UTC().Add(30 * time.Second).Format(time.RFC3339),
	}
	wrapped := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/tenants/ten1/jobs/job1/trigger" {
			fmt.Fprint(w, `{}`) // no task_id
			return
		}
		srv.handler().ServeHTTP(w, r)
	}))
	defer wrapped.Close()

	_, err := runOffboard(t, wrapped.URL, "res1", "--tenant", "ten1", "--poll-interval", "10ms")
	if err == nil || !strings.Contains(err.Error(), "NOT unprotecting") {
		t.Fatalf("expected a refusal without a task handle, got %v", err)
	}
	if srv.unprotected {
		t.Fatal("unprotect must NOT be called when the final backup cannot be confirmed")
	}
}

func TestOffboardPathEscapesCraftedIDs(t *testing.T) {
	// IDs with path-reserved characters must arrive percent-encoded so the
	// verify-then-DELETE invariant cannot be split by a crafted ID. The
	// recording server captures every EscapedPath; the crafted jobID and
	// resourceID must appear encoded (%2F, %3F, %20) and never raw.
	craftedRes := "res 1/x"
	craftedJob := "job/1?x"
	fresh := time.Now().UTC().Add(30 * time.Second).Format(time.RFC3339)
	var mu sync.Mutex
	var paths []string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ep := r.URL.EscapedPath()
		mu.Lock()
		paths = append(paths, r.Method+" "+ep)
		mu.Unlock()
		switch {
		case strings.Contains(ep, "/protections"):
			body, _ := json.Marshal(map[string]any{
				"items": []map[string]any{{"id": "prot1", "resource_id": craftedRes, "policy_id": "pol1", "job_id": craftedJob}},
			})
			w.Write(body)
		case strings.Contains(ep, "/jobs/"):
			fmt.Fprint(w, `{"task_id":"task1"}`)
		case strings.Contains(ep, "/tasks/"):
			fmt.Fprint(w, `{"id":"task1","status":"done"}`)
		case strings.Contains(ep, "/archives"):
			body, _ := json.Marshal(map[string]any{
				"items": []map[string]any{{"id": "arc1", "resource_id": craftedRes, "created_at": fresh}},
			})
			w.Write(body)
		default:
			fmt.Fprint(w, `{}`)
		}
	}))
	defer ts.Close()

	out, err := runOffboard(t, ts.URL, craftedRes, "--tenant", "ten1", "--poll-interval", "10ms")
	if err != nil {
		t.Fatalf("offboard with crafted IDs: %v\n%s", err, out)
	}
	mu.Lock()
	defer mu.Unlock()
	sawEncodedJob, sawEncodedRes := false, false
	for _, p := range paths {
		if strings.Contains(p, "job%2F1%3Fx") {
			sawEncodedJob = true
		}
		if strings.Contains(p, "res%201%2Fx") {
			sawEncodedRes = true
		}
		if strings.Contains(p, "/jobs/job/1") || strings.Contains(p, "/resources/res 1/x") {
			t.Errorf("raw (unescaped) crafted ID reached the server: %s", p)
		}
	}
	if !sawEncodedJob {
		t.Errorf("trigger path never carried the percent-encoded jobID; paths: %v", paths)
	}
	if !sawEncodedRes {
		t.Errorf("unprotect path never carried the percent-encoded resourceID; paths: %v", paths)
	}
}
