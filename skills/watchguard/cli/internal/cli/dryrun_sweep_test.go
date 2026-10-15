// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"crypto/sha256"
	"encoding/hex"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"testing"

	"github.com/spf13/cobra"
)

type requestRecorder struct {
	mu   sync.Mutex
	seen []string
}

func (r *requestRecorder) handler(w http.ResponseWriter, req *http.Request) {
	r.mu.Lock()
	r.seen = append(r.seen, req.Method+" "+req.URL.Path)
	r.mu.Unlock()
	w.Header().Set("Content-Type", "application/json")
	switch {
	case strings.Contains(req.URL.Path, "/oauth/token"):
		_, _ = w.Write([]byte(`{"access_token":"tok","expires_in":3600}`))
	case strings.HasSuffix(req.URL.Path, "/audiences"):
		_, _ = w.Write([]byte(`{"audience":"aud-x"}`))
	default:
		_, _ = w.Write([]byte(`{"results":[],"objects":[],"data":[],"count":0}`))
	}
}

func (r *requestRecorder) count() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.seen)
}

func (r *requestRecorder) since(n int) []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]string(nil), r.seen[n:]...)
}

// homeSnapshot hashes every file under dir except lock files and the SQLite
// reader sidecars a read-only open creates next to a WAL database (-shm and
// an empty -wal): the database file itself must not change.
func homeSnapshot(t *testing.T, dir string) map[string]string {
	t.Helper()
	out := map[string]string{}
	_ = filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		if strings.HasSuffix(p, ".lock") || strings.HasSuffix(p, "-shm") {
			return nil
		}
		b, rerr := os.ReadFile(p)
		if rerr != nil {
			return nil
		}
		if strings.HasSuffix(p, "-wal") && len(b) == 0 {
			return nil
		}
		sum := sha256.Sum256(b)
		out[strings.TrimPrefix(p, dir)] = hex.EncodeToString(sum[:])
		return nil
	})
	return out
}

// runExecute drives the real entrypoint (Execute reads os.Args) with stdin
// holding a small JSON body and stdout/stderr discarded.
func runExecute(t *testing.T, args []string) {
	t.Helper()
	prevArgs, prevIn, prevOut, prevErr := os.Args, os.Stdin, os.Stdout, os.Stderr
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	_, _ = w.WriteString("{\"name\":\"x\"}\n")
	_ = w.Close()
	devnull, _ := os.OpenFile(os.DevNull, os.O_WRONLY, 0)
	os.Args, os.Stdin, os.Stdout, os.Stderr = append([]string{"watchguard-cli"}, args...), r, devnull, devnull
	defer func() {
		os.Args, os.Stdin, os.Stdout, os.Stderr = prevArgs, prevIn, prevOut, prevErr
		_ = r.Close()
		_ = devnull.Close()
	}()
	_ = Execute()
}

// dryRunCommandArgs builds an invocation for every runnable command: a
// placeholder for each positional in Use, --stdin where offered, and the
// inputs the honoring commands need.
func dryRunCommandArgs(root *cobra.Command, input string) [][]string {
	var out [][]string
	var walk func(c *cobra.Command, path []string)
	walk = func(c *cobra.Command, path []string) {
		for _, child := range c.Commands() {
			p := append(append([]string{}, path...), child.Name())
			if child.Runnable() && child.Name() != "help" && (len(p) == 0 || p[0] != "completion") {
				args := append([]string{}, p...)
				for _, tok := range strings.Fields(child.Use)[1:] {
					if strings.HasPrefix(tok, "<") || strings.HasPrefix(tok, "[") {
						args = append(args, "x")
					}
				}
				switch strings.Join(p, " ") {
				case "auth login":
					args = append(args, "--client-id", "NEWID", "--client-secret", "NEWSECRET")
				case "feedback":
					args = append(args, "--send")
				case "import":
					args = []string{"import", "firebox-aliases", "--input", input}
				}
				if child.Flags().Lookup("stdin") != nil {
					args = append(args, "--stdin")
				}
				out = append(out, append(args, "--dry-run", "--json"))
			}
			walk(child, p)
		}
	}
	walk(root, nil)
	return out
}

// hand-fix (handfixes.json dry-run-never-mints, dry-run-side-effect-free):
// every command under --dry-run sends no request (no token, no API call, no
// feedback or webhook POST) and changes no local file (credentials, config,
// profiles, job log, feedback log, the store, the learn journal), on a fresh
// home, with a cached token bound to another Access ID, with a different
// token URL, with WATCHGUARD_AUDIENCE set, and over an existing synced store.
func TestDryRunIsSideEffectFreeEverywhere(t *testing.T) {
	isolateAuthEnv(t)
	rec := &requestRecorder{}
	srv := httptest.NewServer(http.HandlerFunc(rec.handler))
	defer srv.Close()
	input := filepath.Join(t.TempDir(), "in.jsonl")
	if err := os.WriteFile(input, []byte("{\"name\":\"x\"}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("WATCHGUARD_FEEDBACK_ENDPOINT", srv.URL+"/feedback")
	t.Setenv("WATCHGUARD_NO_LEARN", "")
	cmds := dryRunCommandArgs(RootCmd(), input)
	if len(cmds) < 200 {
		t.Fatalf("only %d commands enumerated", len(cmds))
	}
	for _, scenario := range []string{"fresh", "bound", "tokenurl", "audience", "synced"} {
		home := t.TempDir()
		t.Setenv("WATCHGUARD_HOME", home)
		t.Setenv("WATCHGUARD_BASE_URL", srv.URL)
		t.Setenv("WATCHGUARD_TOKEN_URL", srv.URL+"/oauth/token")
		t.Setenv("WATCHGUARD_CLIENT_ID", "RW")
		t.Setenv("WATCHGUARD_CLIENT_SECRET", "rwsec")
		t.Setenv("WATCHGUARD_API_KEY", "KEY")
		t.Setenv("WATCHGUARD_ACCOUNT_ID", "ACC-1")
		t.Setenv("WATCHGUARD_AUDIENCE", "")
		if scenario != "fresh" {
			runExecute(t, []string{"auth", "login", "--client-id", "RW", "--client-secret", "rwsec"})
		}
		switch scenario {
		case "bound":
			t.Setenv("WATCHGUARD_CLIENT_ID", "RO")
			t.Setenv("WATCHGUARD_CLIENT_SECRET", "rosec")
		case "tokenurl":
			t.Setenv("WATCHGUARD_TOKEN_URL", srv.URL+"/oauth/token2")
		case "audience":
			t.Setenv("WATCHGUARD_AUDIENCE", "aud-x")
		case "synced":
			runExecute(t, []string{"sync", "--full"})
			runExecute(t, []string{"profile", "save", "p1"})
		}
		var offenders []string
		for _, args := range cmds {
			before, n := homeSnapshot(t, home), rec.count()
			runExecute(t, args)
			after := homeSnapshot(t, home)
			var changed []string
			for k := range after {
				if before[k] != after[k] {
					changed = append(changed, k)
				}
			}
			for k := range before {
				if _, ok := after[k]; !ok {
					changed = append(changed, k+" (deleted)")
				}
			}
			sort.Strings(changed)
			if reqs := rec.since(n); len(reqs) > 0 || len(changed) > 0 {
				offenders = append(offenders, strings.Join(args[:len(args)-2], " ")+": requests="+strings.Join(reqs, ",")+" changed="+strings.Join(changed, ","))
			}
		}
		for _, o := range offenders {
			t.Errorf("%s: --dry-run had a side effect: %s", scenario, o)
		}
	}
}
