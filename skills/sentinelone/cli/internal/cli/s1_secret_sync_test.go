// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
//
// Hand-written (NOT generated): proves a credential the SentinelOne API
// returns never lands on disk through sync, workflow archive or a read's
// write-through, while an operator's own explicit read still prints it.

package cli

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"sentinelone-pp-cli/internal/cliutil/testenv"
)

const (
	secretPassphrase   = "ZEBRA QUARTZ MANGO PIXEL ORBIT NECTAR"
	secretSiteToken    = "s1sitetokenqw7er8ty9ui0op"
	secretGroupToken   = "s1grouptokenzx1cv2bn3m4"
	secretRangerPw     = "s1rangercredpwlk5jh6gf7"
	secretRangerSNMP   = "s1rangersnmpcommunityvb8n"
	secretUninstallPw  = "s1uninstallpwas9df0gh1"
	secretLoginSession = "s1loginsessiontokenpo2iu3"
)

// secretSentinels are matched case-insensitively against every file the run
// leaves behind, word by word too: the FTS index stores lower-cased terms.
var secretSentinels = []string{
	secretPassphrase, "zebra", "quartz", "mango", "pixel", "orbit", "nectar",
	secretSiteToken, secretGroupToken, secretRangerPw, secretRangerSNMP,
	secretUninstallPw, secretLoginSession,
}

type secretStub struct {
	srv  *httptest.Server
	mu   sync.Mutex
	hits map[string]int
}

func (s *secretStub) count(path string) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.hits[path]
}

func (s *secretStub) total() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	n := 0
	for _, v := range s.hits {
		n += v
	}
	return n
}

func (s *secretStub) reset() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.hits = map[string]int{}
}

func newSecretStub(t *testing.T) *secretStub {
	t.Helper()
	st := &secretStub{hits: map[string]int{}}
	st.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		st.mu.Lock()
		st.hits[r.URL.Path]++
		st.mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		page := `"pagination":{"nextCursor":null,"totalItems":1}`
		switch r.URL.Path {
		case "/agents/passphrases":
			_, _ = w.Write([]byte(`{"data":[{"id":"11","uuid":"u-11","computerName":"WS1","passphrase":"` + secretPassphrase + `"}],` + page + `}`))
		case "/sites":
			_, _ = w.Write([]byte(`{"data":{"allSites":{"activeLicenses":1,"totalLicenses":10},"sites":[{"id":"s1","name":"Ordinary Site One","accountId":"acc1","registrationToken":"` + secretSiteToken + `"}]},` + page + `}`))
		case "/sites/s1":
			_, _ = w.Write([]byte(`{"data":{"id":"s1","name":"Ordinary Site One","accountId":"acc1","registrationToken":"` + secretSiteToken + `"}}`))
		case "/groups":
			_, _ = w.Write([]byte(`{"data":[{"id":"g1","name":"Ordinary Group","siteId":"s1","registrationToken":"` + secretGroupToken + `"}],` + page + `}`))
		case "/ranger/cred-groups/details":
			_, _ = w.Write([]byte(`{"data":[{"id":"c1","title":"deploy creds","credType":"snmp","community":"` + secretRangerSNMP + `","password":"` + secretRangerPw + `"}],` + page + `}`))
		case "/accounts/123/uninstall-password/view":
			_, _ = w.Write([]byte(`{"data":{"password":"` + secretUninstallPw + `"}}`))
		case "/users/login/by-token":
			_, _ = w.Write([]byte(`{"data":{"token":"` + secretLoginSession + `","user":{"id":"u1","fullName":"Ordinary User"}}}`))
		default:
			_, _ = w.Write([]byte(`{"data":[],"pagination":{"nextCursor":null,"totalItems":0}}`))
		}
	}))
	t.Cleanup(st.srv.Close)
	return st
}

func runSecretCLI(t *testing.T, args ...string) (string, error) {
	t.Helper()
	root := RootCmd()
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetArgs(args)
	err := root.Execute()
	return out.String(), err
}

// leakedSentinels walks every regular file under dirs (the store, its WAL, the
// response cache, the state dir) and reports which sentinels any file holds.
func leakedSentinels(t *testing.T, dirs ...string) []string {
	t.Helper()
	var leaks []string
	for _, dir := range dirs {
		_ = filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
			if err != nil || !info.Mode().IsRegular() {
				return nil
			}
			b, rerr := os.ReadFile(filepath.Clean(path))
			if rerr != nil {
				return nil
			}
			low := bytes.ToLower(b)
			for _, s := range secretSentinels {
				if bytes.Contains(low, []byte(strings.ToLower(s))) {
					leaks = append(leaks, s+" in "+path)
				}
			}
			return nil
		})
	}
	return leaks
}

func fileTreeContains(t *testing.T, dir, needle string) bool {
	t.Helper()
	found := false
	_ = filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil || !info.Mode().IsRegular() || found {
			return nil
		}
		if b, rerr := os.ReadFile(filepath.Clean(path)); rerr == nil && bytes.Contains(b, []byte(needle)) {
			found = true
		}
		return nil
	})
	return found
}

// TestSyncNeverStoresSecretResources: a default sync must not request the
// agent passphrases or the Ranger credential details, must keep the ordinary
// site and group rows without their registration tokens, and naming a secret
// resource explicitly must be refused before any request.
func TestSyncNeverStoresSecretResources(t *testing.T) {
	home := testenv.Isolate(t)
	st := newSecretStub(t)
	t.Setenv("SENTINELONE_BASE_URL", st.srv.URL)
	t.Setenv("SENTINELONE_API_TOKEN", "test-token-for-secret-sync")
	dbDir := t.TempDir()
	db := filepath.Join(dbDir, "s1.db")

	if out, err := runSecretCLI(t, "sync", "--db", db, "--json"); err != nil {
		t.Fatalf("default sync failed: %v\n%s", err, out)
	}
	for _, p := range []string{"/agents/passphrases", "/ranger/cred-groups/details"} {
		if n := st.count(p); n != 0 {
			t.Errorf("default sync requested %s %d time(s); a credential endpoint is never synced", p, n)
		}
	}
	if st.count("/sites") == 0 || st.count("/groups") == 0 {
		t.Fatalf("default sync skipped sites/groups; hits=%v", st.hits)
	}
	if !fileTreeContains(t, dbDir, "Ordinary Site One") || !fileTreeContains(t, dbDir, "Ordinary Group") {
		t.Fatalf("ordinary site/group rows were not stored")
	}

	// Explicitly named: refused with a non-zero exit before any request.
	for _, named := range []string{"agents-passphrases", "ranger", "sites,agents-passphrases"} {
		st.reset()
		out, err := runSecretCLI(t, "sync", "--resources", named, "--db", db, "--json")
		if err == nil {
			t.Errorf("sync --resources %s exited 0; a credential resource must be refused\n%s", named, out)
		}
		if n := st.total(); n != 0 {
			t.Errorf("sync --resources %s sent %d request(s) before refusing: %v", named, n, st.hits)
		}
		msg := strings.ToLower(out)
		if err != nil {
			msg += " " + strings.ToLower(err.Error())
		}
		if !strings.Contains(msg, "credential") || !strings.Contains(msg, "never stored") {
			t.Errorf("sync --resources %s refusal does not say why: %q", named, msg)
		}
	}

	// workflow archive walks the same resource list and must skip them too.
	st.reset()
	archiveDir := t.TempDir()
	if out, err := runSecretCLI(t, "workflow", "archive", "--db", filepath.Join(archiveDir, "a.db"), "--json"); err != nil {
		t.Fatalf("workflow archive failed: %v\n%s", err, out)
	}
	for _, p := range []string{"/agents/passphrases", "/ranger/cred-groups/details"} {
		if n := st.count(p); n != 0 {
			t.Errorf("workflow archive requested %s %d time(s)", p, n)
		}
	}

	// An operator's own explicit reads still print the secret (the class is
	// persistence, not the operator's terminal), but nothing reaches the store
	// (write-through) or the response cache.
	for _, tc := range []struct {
		args []string
		want string
	}{
		{[]string{"agents", "get-passphrase", "--json"}, secretPassphrase},
		{[]string{"sites", "get-by-id", "s1", "--json"}, secretSiteToken},
		{[]string{"accounts", "uninstall-password", "get", "123", "--json"}, secretUninstallPw},
		{[]string{"users", "login-by-token", "--token", "operator-supplied-input", "--json"}, secretLoginSession},
		{[]string{"ranger", "get-cred-group-details", "--json"}, secretRangerPw},
	} {
		out, err := runSecretCLI(t, tc.args...)
		if err != nil {
			t.Fatalf("%v failed: %v\n%s", tc.args, err, out)
		}
		if !strings.Contains(out, tc.want) {
			t.Errorf("%v no longer shows the operator the value they asked for:\n%s", tc.args, out)
		}
	}

	if leaks := leakedSentinels(t, dbDir, archiveDir, home); len(leaks) > 0 {
		t.Fatalf("credentials persisted on disk:\n%s", strings.Join(leaks, "\n"))
	}
}
