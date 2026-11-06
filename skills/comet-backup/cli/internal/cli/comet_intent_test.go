// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

// comet-backup handfix (rerun-dispatch-intent): a run stopped between sending a
// dispatch and hearing back leaves the backup in doubt. The next run does not send
// it again until the server has had time to show the job.

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"comet-backup-pp-cli/internal/cliutil"
)

// intentServer is one seed's scenario with faults off. Before each dispatch answer
// it runs reply, which may answer for itself and report true.
func intentServer(t *testing.T, reply func(w http.ResponseWriter, r *http.Request) bool) (*simComet, []string) {
	t.Helper()
	simIsolate(t)
	s := newSimComet(1)
	s.buildRerunScenario()
	s.faultFree = true
	want := s.rerunnable()
	if len(want) == 0 {
		t.Fatal("scenario has nothing to re-run")
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/dispatcher/run-backup") && reply != nil && reply(w, r) {
			return
		}
		s.ServeHTTP(w, r)
	}))
	t.Cleanup(srv.Close)
	if err := os.Setenv("COMET_BACKUP_BASE_URL", srv.URL); err != nil {
		t.Fatal(err)
	}
	return s, want
}

func simIntentKey(t *testing.T, s *simComet, rerunnable string) string {
	t.Helper()
	tenant, rule, ok := strings.Cut(rerunnable, "/")
	if !ok {
		t.Fatalf("unexpected rerunnable %q", rerunnable)
	}
	tn := s.tenants[tenant]
	sd := tn.rules[rule]
	job, _ := s.newest(tenant, sd[0], sd[1])
	return rerunIntentKey(tenant, tn.sources[sd[0]], rule, job.GUID)
}

func clearIntents(t *testing.T, s *simComet) {
	t.Cleanup(func() {
		for _, j := range s.jobs {
			cliutil.ClearDispatchIntent(rerunIntentKey(j.User, j.Device, j.Rule, j.GUID))
		}
	})
}

func TestInterruptedDispatchIsNotSentAgainUntilTheHoldPasses(t *testing.T) {
	if inFreshProcess(t) {
		return
	}
	s, want := intentServer(t, nil)
	clearIntents(t, s)
	rerunIntentHold = time.Hour
	key := simIntentKey(t, s, want[0])
	if err := cliutil.RecordDispatchIntent(key); err != nil {
		t.Fatal(err)
	}
	exit, out := simRun(context.Background(), rerunArgs("5s", false)...)
	if exit == 0 || !strings.Contains(out, "unconfirmed") {
		t.Fatalf("a run with a backup in doubt exited %d:\n%s", exit, out)
	}
	s.mu.Lock()
	left := s.rerunnable()
	s.mu.Unlock()
	found := false
	for _, l := range left {
		found = found || l == want[0]
	}
	if !found {
		t.Fatalf("the backup in doubt was sent again; still re-runnable: %v", left)
	}
	rerunIntentHold = 0
	if exit, out := simRun(context.Background(), rerunArgs("5s", false)...); exit != 0 {
		t.Fatalf("after the hold the run exited %d:\n%s", exit, out)
	}
	s.mu.Lock()
	left = s.rerunnable()
	s.mu.Unlock()
	if len(left) != 0 {
		t.Fatalf("after the hold still re-runnable: %v", left)
	}
}

func TestDispatchIntentSurvivesALostReplyOnly(t *testing.T) {
	if inFreshProcess(t) {
		return
	}
	inFlight := map[string]bool{}
	var mode string
	s, want := intentServer(t, nil)
	clearIntents(t, s)
	rerunIntentHold = time.Hour
	key := simIntentKey(t, s, want[0])
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/dispatcher/run-backup") {
			_, found, err := cliutil.DispatchIntentAge(key)
			if err != nil {
				t.Error(err)
			}
			inFlight[mode] = inFlight[mode] || found
			switch mode {
			case "drop":
				if hj, ok := w.(http.Hijacker); ok {
					if c, _, err := hj.Hijack(); err == nil {
						_ = c.Close()
					}
				}
				return
			case "refuse":
				w.WriteHeader(http.StatusBadRequest)
				return
			}
		}
		s.ServeHTTP(w, r)
	}))
	t.Cleanup(srv.Close)
	if err := os.Setenv("COMET_BACKUP_BASE_URL", srv.URL); err != nil {
		t.Fatal(err)
	}
	mode = "refuse"
	if exit, _ := simRun(context.Background(), rerunArgs("5s", false)...); exit == 0 {
		t.Fatal("a refused dispatch exited 0")
	}
	if _, found, _ := cliutil.DispatchIntentAge(key); found {
		t.Error("a refused dispatch stayed in doubt")
	}
	mode = "drop"
	exit, out := simRun(context.Background(), rerunArgs("5s", false)...)
	if exit == 0 || !strings.Contains(out, "unconfirmed") {
		t.Fatalf("a lost reply exited %d:\n%s", exit, out)
	}
	if _, found, _ := cliutil.DispatchIntentAge(key); !found {
		t.Error("a lost reply was not kept in doubt")
	}
	if !inFlight["refuse"] || !inFlight["drop"] {
		t.Errorf("the intent was not on disk before the send: %v", inFlight)
	}
}
