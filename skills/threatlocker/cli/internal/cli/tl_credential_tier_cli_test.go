// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
//
// Hand-authored: the credential tier on the CLI surface. Two ThreatLocker reads
// answer with enrollment credential material rather than inventory:
//
//   organizations auth-key  GET /Organization/OrganizationGetAuthKeyById -> {organizationId, authKey}
//   computers install-info  GET /Computer/ComputerGetForNewComputer       -> {installKey, downloadUrl}
//
// Either value enrolls a new agent into the tenant. The operator who types the
// command in a terminal gets the value on stdout, and that is the only place it
// may go: never the on-disk response cache, never the local SQLite store, and
// never a process the MCP server spawned. The member list below is this test's
// own oracle, deliberately not read from the production allowlist, so a
// shrinking allowlist fails here instead of passing silently.

package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"threatlocker-pp-cli/internal/cliutil/testenv"
)

type tlCredentialMember struct {
	command []string
	path    string
	body    func(secret string) map[string]any
}

var tlCredentialMembers = []tlCredentialMember{
	{
		command: []string{"organizations", "auth-key"},
		path:    "/Organization/OrganizationGetAuthKeyById",
		body: func(secret string) map[string]any {
			return map[string]any{"organizationId": "org-credential-fixture", "authKey": secret}
		},
	},
	{
		command: []string{"computers", "install-info"},
		path:    "/Computer/ComputerGetForNewComputer",
		body: func(secret string) map[string]any {
			return map[string]any{"installKey": secret, "downloadUrl": "https://download.example.test/agent?key=" + secret}
		},
	},
}

// tlCredentialServer answers every request with body(path) and counts hits
// per path, so a test can tell a cache hit (no wire request) from a fetch.
func tlCredentialServer(t *testing.T, body func(path string) any) (string, func(path string) int64) {
	t.Helper()
	var hits = map[string]*int64{}
	for _, m := range tlCredentialMembers {
		hits[m.path] = new(int64)
	}
	var other int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if n, ok := hits[r.URL.Path]; ok {
			atomic.AddInt64(n, 1)
		} else {
			atomic.AddInt64(&other, 1)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(body(r.URL.Path))
	}))
	t.Cleanup(srv.Close)
	return srv.URL, func(path string) int64 {
		if n, ok := hits[path]; ok {
			return atomic.LoadInt64(n)
		}
		return atomic.LoadInt64(&other)
	}
}

// runTLCredential runs one command in an already-isolated home. It does not
// call testenv.Isolate itself, so two runs share one cache and one store.
func runTLCredential(t *testing.T, baseURL string, args ...string) (string, error) {
	t.Helper()
	t.Setenv("THREATLOCKER_API_KEY", "0000000000000000000000000000000000000000000000000000000000000002")
	t.Setenv("THREATLOCKER_ORG_ID", "")
	t.Setenv("THREATLOCKER_BASE_URL", baseURL)
	cmd := RootCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs(args)
	err := cmd.ExecuteContext(context.Background())
	return out.String(), err
}

// filesContaining walks root and returns every regular file whose bytes hold
// needle. SQLite stores TEXT as plain UTF-8, so a value written to the store
// (main file, -wal or -journal) shows up here as well as a cache body does.
func filesContaining(t *testing.T, root, needle string) []string {
	t.Helper()
	var hits []string
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if !info.Mode().IsRegular() {
			return nil
		}
		data, readErr := os.ReadFile(path) // #nosec G304 -- test sandbox walk.
		if readErr == nil && bytes.Contains(data, []byte(needle)) {
			rel, _ := filepath.Rel(root, path)
			hits = append(hits, rel)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk %s: %v", root, err)
	}
	return hits
}

// The detector must be able to see a write before its silence means anything.
// An ordinary inventory read is cached and stored, so its marker lands on disk.
func TestCredentialTierDiskScanSeesAnOrdinaryRead(t *testing.T) {
	home := testenv.Isolate(t)
	const marker = "ordinary-group-marker-7f3e2a"
	base, _ := tlCredentialServer(t, func(string) any {
		return []map[string]any{{"computerGroupId": "g-1", "name": marker}}
	})
	if out, err := runTLCredential(t, base, "computer-groups", "list", "--json"); err != nil {
		t.Fatalf("computer-groups list: %v\n%s", err, out)
	}
	if hits := filesContaining(t, home, marker); len(hits) == 0 {
		t.Fatal("an ordinary read left nothing on disk, so the credential-tier scan below proves nothing")
	}
}

// Failing input (2): `threatlocker-cli organizations auth-key` stored the
// response, AuthKey included, as raw JSON in the response cache, and wrote it
// through to the SQLite store (organizations.auth_key). The operator must still
// get the key on stdout; nothing may land on disk, and a second run must reach
// the API again because there is no cache entry to answer it.
func TestCredentialTierCLIReadNeverCachesOrStoresTheSecret(t *testing.T) {
	for _, m := range tlCredentialMembers {
		m := m
		t.Run(strings.Join(m.command, "_"), func(t *testing.T) {
			home := testenv.Isolate(t)
			secret := "SENTINEL-" + strings.ReplaceAll(strings.Join(m.command, "-"), "-", "") + "-c0ffee5ecret"
			base, hits := tlCredentialServer(t, func(path string) any {
				if path == m.path {
					return m.body(secret)
				}
				return []any{}
			})
			args := append(append([]string{}, m.command...), "--json")
			out, err := runTLCredential(t, base, args...)
			if err != nil {
				t.Fatalf("%s: %v\n%s", strings.Join(m.command, " "), err, out)
			}
			if !strings.Contains(out, secret) {
				t.Fatalf("the operator asked for the credential in a terminal and did not get it:\n%s", out)
			}
			if got := hits(m.path); got != 1 {
				t.Fatalf("first run reached %s %d time(s), want 1", m.path, got)
			}
			if leaked := filesContaining(t, home, secret); len(leaked) > 0 {
				t.Fatalf("credential written to disk under the CLI home: %v", leaked)
			}
			out, err = runTLCredential(t, base, args...)
			if err != nil {
				t.Fatalf("second run: %v\n%s", err, out)
			}
			if got := hits(m.path); got != 2 {
				t.Fatalf("second run was answered without reaching the API (%d wire request(s)); the credential was served from a cache", got)
			}
			if leaked := filesContaining(t, home, secret); len(leaked) > 0 {
				t.Fatalf("credential written to disk after a second run: %v", leaked)
			}
		})
	}
}

// A process the MCP server spawned (THREATLOCKER_MCP_CHILD=1, set by every
// cobratree shell-out) is an MCP client by another name. It must refuse a
// credential-tier read before anything reaches the wire, and point the
// operator at the terminal.
func TestCredentialTierCLIRefusesInsideAnMCPChild(t *testing.T) {
	for _, m := range tlCredentialMembers {
		m := m
		t.Run(strings.Join(m.command, "_"), func(t *testing.T) {
			home := testenv.Isolate(t)
			secret := "SENTINEL-mcpchild-" + strings.Join(m.command, "") + "-0ddba11"
			base, hits := tlCredentialServer(t, func(path string) any {
				if path == m.path {
					return m.body(secret)
				}
				return []any{}
			})
			t.Setenv("THREATLOCKER_MCP_CHILD", "1")
			out, err := runTLCredential(t, base, append(append([]string{}, m.command...), "--json")...)
			if err == nil {
				t.Fatalf("an MCP child ran %s and returned:\n%s", strings.Join(m.command, " "), out)
			}
			if strings.Contains(out, secret) || strings.Contains(err.Error(), secret) {
				t.Fatalf("an MCP child printed the credential:\n%s\n%v", out, err)
			}
			if !strings.Contains(err.Error()+out, "terminal") {
				t.Fatalf("refusal does not tell the operator to use a terminal: %v\n%s", err, out)
			}
			if got := hits(m.path); got != 0 {
				t.Fatalf("an MCP child reached %s %d time(s)", m.path, got)
			}
			if leaked := filesContaining(t, home, secret); len(leaked) > 0 {
				t.Fatalf("credential on disk: %v", leaked)
			}
		})
	}
}

// doctor's credential probe walks the tree for any argument-free GET. A
// credential-tier read is argument-free and GET, so it qualifies as an
// ordinary read unless the tier excludes it: a health check must never
// fetch an enrollment key.
func TestDoctorNeverProbesACredentialTierRead(t *testing.T) {
	testenv.Isolate(t)
	root := RootCmd()
	for _, m := range tlCredentialMembers {
		leaf, _, err := root.Find(m.command)
		if err != nil || leaf == nil || leaf.Name() != m.command[len(m.command)-1] {
			t.Fatalf("command %q not found: %v", strings.Join(m.command, " "), err)
		}
		if p := doctorProbeableLeaf(leaf); p != "" {
			t.Errorf("doctor may probe %q (%s), a credential-returning read", strings.Join(m.command, " "), p)
		}
	}
}
