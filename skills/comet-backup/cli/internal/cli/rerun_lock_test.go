// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

// comet-backup handfix (rerun-dispatch-lock): the rerun-failed dispatch lock
// is one lock per OS account, wherever the invocation environment points.
// Codex: two concurrent rerun-failed --yes runs with identical credentials and
// server but HOME=/tmp/comet-a and HOME=/tmp/comet-b took different locks,
// both planned the same failed scheduled job, and both dispatched it.

package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"comet-backup-pp-cli/internal/client"
	"comet-backup-pp-cli/internal/cliutil"
	"comet-backup-pp-cli/internal/config"
)

// rerunLockEnvVars is every environment variable a path lookup on any
// platform reads: home, XDG, temp, Windows profile folders and this CLI's
// own overrides. None of them may move or rename the dispatch lock.
var rerunLockEnvVars = []string{
	"HOME", "USERPROFILE", "HOMEDRIVE", "HOMEPATH",
	"XDG_CONFIG_HOME", "XDG_DATA_HOME", "XDG_STATE_HOME", "XDG_CACHE_HOME", "XDG_RUNTIME_DIR",
	"TMPDIR", "TMP", "TEMP", "LOCALAPPDATA", "APPDATA",
	"COMET_BACKUP_HOME", "COMET_BACKUP_CONFIG", "COMET_BACKUP_CONFIG_DIR",
	"COMET_BACKUP_DATA_DIR", "COMET_BACKUP_STATE_DIR", "COMET_BACKUP_CACHE_DIR",
}

// rerunLockURLSpellings are one Comet Server written several ways.
var rerunLockURLSpellings = []string{
	"https://BACKUP.example.com",
	"https://backup.example.com/",
	"https://backup.example.com:443",
	"https://backup.example.com:443/",
	"HTTPS://Backup.Example.Com//",
	"https://backup.example.com/api/v1/",
}

// TestRerunDispatchLockIgnoresInvocationEnvironment holds the lock under one
// environment, then tries it again with each variable (and --home, and each
// URL spelling) changed: every attempt must wait out its deadline without
// running, never take a second lock.
func TestRerunDispatchLockIgnoresInvocationEnvironment(t *testing.T) {
	simIsolate(t)
	holding, release := make(chan struct{}), make(chan struct{})
	done := make(chan error, 1)
	first := client.New(&config.Config{BaseURL: "https://backup.example.com"}, time.Second, 0)
	go func() {
		done <- withRerunDispatchLock(context.Background(), first, func() error {
			close(holding)
			<-release
			return nil
		})
	}()
	select {
	case <-holding:
	case err := <-done:
		t.Fatalf("the first run could not take the lock: %v", err)
	case <-time.After(10 * time.Second):
		t.Fatal("the first run did not take the lock")
	}
	defer func() {
		close(release)
		if err := <-done; err != nil {
			t.Errorf("the first run: %v", err)
		}
	}()

	type variant struct {
		name string
		env  map[string]string
		home string // --home
		url  string
	}
	var variants []variant
	all := map[string]string{}
	for _, name := range rerunLockEnvVars {
		dir := t.TempDir()
		variants = append(variants, variant{name: name, env: map[string]string{name: dir}})
		all[name] = t.TempDir()
	}
	variants = append(variants,
		variant{name: "every variable at once", env: all},
		variant{name: "--home", home: t.TempDir()},
		variant{name: "everything and --home", env: all, home: t.TempDir(), url: "https://backup.example.com:443/"},
	)
	for _, u := range rerunLockURLSpellings {
		variants = append(variants, variant{name: "url " + u, url: u})
	}
	for _, v := range variants {
		t.Run(v.name, func(t *testing.T) {
			for k, val := range v.env {
				t.Setenv(k, val)
			}
			if v.home != "" {
				restore, err := cliutil.SetHomeOverride(v.home)
				if err != nil {
					t.Fatal(err)
				}
				defer restore()
			}
			u := v.url
			if u == "" {
				u = "https://backup.example.com"
			}
			other := client.New(&config.Config{BaseURL: u}, time.Second, 0)
			ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
			defer cancel()
			ran := false
			err := withRerunDispatchLock(ctx, other, func() error { ran = true; return nil })
			if ran {
				t.Fatalf("a second run took the dispatch lock while the first held it: %s splits the lock", v.name)
			}
			if ExitCode(err) != 7 {
				t.Fatalf("err = %v (exit %d), want the lock-wait timeout (exit 7)", err, ExitCode(err))
			}
		})
	}
}

// rerunLockChild is one rerun-failed --yes run in its own process: the
// variables it sets after the test sandbox, and its CLI arguments.
type rerunLockChild struct {
	Env    map[string]string `json:"env"`
	Args   []string          `json:"args"`
	Result string            `json:"result"`
}

const rerunLockChildVar = "COMET_RERUN_LOCK_CHILD"

// TestRerunLockChild is the child process body for
// TestRerunFailedDispatchesOnceAcrossInvocationEnvironments.
func TestRerunLockChild(t *testing.T) {
	raw := os.Getenv(rerunLockChildVar)
	if raw == "" {
		t.Skip("runs only as a child process of TestRerunFailedDispatchesOnceAcrossInvocationEnvironments")
	}
	var spec rerunLockChild
	if err := json.Unmarshal([]byte(raw), &spec); err != nil {
		t.Fatal(err)
	}
	simIsolate(t)
	for k, v := range spec.Env {
		t.Setenv(k, v)
	}
	exit, out := simRun(context.Background(), spec.Args...)
	b, err := json.Marshal(map[string]any{"exit": exit, "out": out})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(spec.Result, b, 0o600); err != nil {
		t.Fatal(err)
	}
}

// jobsBarrier holds the first job-list read until a second run reads it too
// (or the wait runs out), so two runs that are not serialized both plan
// before either dispatches: the race the lock exists to prevent, made certain.
type jobsBarrier struct {
	mu   sync.Mutex
	n    int
	both chan struct{}
}

const jobsBarrierWait = 2 * time.Second

func (b *jobsBarrier) wrap(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/get-jobs-for-date-range") {
			b.mu.Lock()
			b.n++
			if b.n == 2 {
				close(b.both)
			}
			b.mu.Unlock()
			select {
			case <-b.both:
			case <-time.After(jobsBarrierWait):
			}
		}
		h.ServeHTTP(w, r)
	})
}

// TestRerunFailedDispatchesOnceAcrossInvocationEnvironments runs two real
// rerun-failed --yes processes at once against one fake Comet Server, with
// the same credentials but a different invocation environment, and counts
// what the server started: each failed scheduled backup exactly once.
func TestRerunFailedDispatchesOnceAcrossInvocationEnvironments(t *testing.T) {
	simIsolate(t)
	base := t.TempDir()
	dirs := func(tag string) map[string]string {
		env := map[string]string{}
		for _, name := range rerunLockEnvVars {
			d := filepath.Join(base, tag, strings.ToLower(name))
			if err := os.MkdirAll(d, 0o700); err != nil {
				t.Fatal(err)
			}
			if name == "COMET_BACKUP_CONFIG" {
				d = filepath.Join(d, "config.toml") // a config file path, not a directory
			}
			env[name] = d
		}
		return env
	}
	homeOnly := func(tag string) map[string]string {
		d := filepath.Join(base, "home-"+tag)
		if err := os.MkdirAll(d, 0o700); err != nil {
			t.Fatal(err)
		}
		return map[string]string{"HOME": d, "USERPROFILE": d}
	}
	mkHome := func(tag string) string {
		d := filepath.Join(base, "flag-home-"+tag)
		if err := os.MkdirAll(d, 0o700); err != nil {
			t.Fatal(err)
		}
		return d
	}
	type side struct {
		env  map[string]string
		home string
		url  func(string) string
	}
	same := func(u string) string { return u }
	pairs := []struct {
		name string
		a, b side
	}{
		{name: "HOME differs", a: side{env: homeOnly("a"), url: same}, b: side{env: homeOnly("b"), url: same}},
		{
			name: "every variable, --home and the URL spelling differ",
			a:    side{env: dirs("a"), home: mkHome("a"), url: same},
			b:    side{env: dirs("b"), home: mkHome("b"), url: func(u string) string { return strings.ToUpper(u[:4]) + u[4:] + "/" }},
		},
	}
	for i, p := range pairs {
		t.Run(p.name, func(t *testing.T) {
			s := newSimComet(int64(9100 + i))
			s.buildRerunScenario()
			s.faultFree = true
			b := &jobsBarrier{both: make(chan struct{})}
			srv := httptest.NewServer(b.wrap(s))
			defer srv.Close()
			s.mu.Lock()
			before := s.rerunnable()
			s.mu.Unlock()

			type result struct {
				Exit int    `json:"exit"`
				Out  string `json:"out"`
			}
			results := make([]result, 2)
			errs := make([]error, 2)
			var wg sync.WaitGroup
			for k, sd := range []side{p.a, p.b} {
				env := map[string]string{"COMET_BACKUP_BASE_URL": sd.url(srv.URL)}
				for name, v := range sd.env {
					env[name] = v
				}
				args := rerunArgs(simCalmTimeout, false)
				if sd.home != "" {
					args = append(args, "--home", sd.home)
				}
				spec := rerunLockChild{Env: env, Args: args, Result: filepath.Join(base, fmt.Sprintf("result-%d-%d.json", i, k))}
				raw, err := json.Marshal(spec)
				if err != nil {
					t.Fatal(err)
				}
				wg.Add(1)
				go func(k int, raw []byte, resultPath string) {
					defer wg.Done()
					cmd := exec.Command(os.Args[0], "-test.run=^TestRerunLockChild$", "-test.count=1")
					cmd.Env = append(os.Environ(), rerunLockChildVar+"="+string(raw))
					if out, err := cmd.CombinedOutput(); err != nil {
						errs[k] = fmt.Errorf("child %d: %v\n%s", k, err, out)
						return
					}
					data, err := os.ReadFile(resultPath)
					if err != nil {
						errs[k] = fmt.Errorf("child %d wrote no result: %v", k, err)
						return
					}
					errs[k] = json.Unmarshal(data, &results[k])
				}(k, raw, spec.Result)
			}
			wg.Wait()
			for _, err := range errs {
				if err != nil {
					t.Fatal(err)
				}
			}
			s.mu.Lock()
			defer s.mu.Unlock()
			for k, r := range results {
				if r.Exit != 0 {
					t.Errorf("run %d exited %d: %.600s", k, r.Exit, r.Out)
				}
			}
			if len(s.violations) > 0 {
				t.Errorf("the server saw a backup dispatched while it was already running: %s", strings.Join(s.violations, "; "))
			}
			for _, p := range before {
				tenant, rule, _ := strings.Cut(p, "/")
				n := 0
				for _, j := range s.jobs {
					if strings.HasPrefix(j.GUID, "job-new-") && j.User == tenant && j.Rule == rule {
						n++
					}
				}
				if n != 1 {
					t.Errorf("%s was started %d times, want exactly once", p, n)
				}
			}
			if t.Failed() {
				t.Logf("trace:\n  %s", strings.Join(s.trace, "\n  "))
			}
		})
	}
}
