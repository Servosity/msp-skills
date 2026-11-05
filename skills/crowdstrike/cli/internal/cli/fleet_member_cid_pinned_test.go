// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

// crowdstrike handfixes member-cid-token-pinned and no-credential-in-files, on
// the Flight Control novel: with the parent client pair in FALCON_CLIENT_ID /
// FALCON_CLIENT_SECRET, `fleet sync` mints one token per child CID (member_cid
// in the token form) and reads each child tenant with that token alone. The
// generated client's token binding mints (or reuses) a token for any client
// pair the environment holds, so a child config that is not pinned
// (client.PinToken) would swap in the PARENT's token and store the parent
// tenant's hosts under the child CID.

import (
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"crowdstrike-pp-cli/internal/cliutil/testenv"
)

const (
	pinParentID     = "pinParentClientId-6b2e9c"
	pinParentSecret = "pinParentClientSecret-1d7a4f3e"
	pinChild        = "c0ffee00child00cid000000000000a1"
)

// flightControl is a Falcon parent tenant with Flight Control: the token
// endpoint mints a parent token for the pair alone and a child token for the
// pair plus member_cid; /mssp/queries/children/v1 lists the one child (parent
// token only); the combined hosts list answers the tenant the bearer token
// belongs to. echo makes every API answer repeat the bearer token it received
// under a name that is not a credential (a real API never would; it proves the
// scans read what the program was sent, not what the fake chose to hide).
type flightControl struct {
	srv  *httptest.Server
	echo bool
	mu   sync.Mutex
	// mints is the member_cid of every accepted token request ("" = parent).
	mints []string
	// reqs is "METHOD path Authorization" of every API request.
	reqs   []string
	minted map[string]string // token -> tenant ("parent" or the child CID)
}

func newFlightControl(t *testing.T, echo bool) *flightControl {
	t.Helper()
	f := &flightControl{echo: echo, minted: map[string]string{}}
	f.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/oauth2/token" {
			raw, _ := io.ReadAll(r.Body)
			form, _ := url.ParseQuery(string(raw))
			if r.Method != http.MethodPost || r.Header.Get("Authorization") != "" || form.Get("client_id") != pinParentID ||
				form.Get("client_secret") != pinParentSecret || form.Get("grant_type") != "client_credentials" {
				w.WriteHeader(http.StatusUnauthorized)
				_, _ = io.WriteString(w, `{"errors":[{"code":401,"message":"access denied, invalid client"}]}`)
				return
			}
			cid := form.Get("member_cid")
			f.mu.Lock()
			f.mints = append(f.mints, cid)
			tenant, tok := "parent", fmt.Sprintf("pinParentToken-%d-8c3f1a", len(f.mints))
			if cid != "" {
				tenant, tok = cid, fmt.Sprintf("pinChildToken-%d-5e9b2d", len(f.mints))
			}
			f.minted[tok] = tenant
			f.mu.Unlock()
			fmt.Fprintf(w, `{"access_token":%q,"token_type":"bearer","expires_in":1799}`, tok)
			return
		}
		auth := r.Header.Get("Authorization")
		tok := strings.TrimPrefix(auth, "Bearer ")
		f.mu.Lock()
		f.reqs = append(f.reqs, r.Method+" "+r.URL.Path+" "+auth)
		tenant, ok := f.minted[tok]
		f.mu.Unlock()
		if !ok {
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = io.WriteString(w, `{"errors":[{"code":401,"message":"access denied, authorization failed"}]}`)
			return
		}
		note := "none"
		if f.echo {
			note = tok
		}
		switch {
		case r.URL.Path == "/mssp/queries/children/v1" && tenant == "parent":
			fmt.Fprintf(w, `{"meta":{"pagination":{"offset":0,"limit":100,"total":1},"note":%q},"resources":[%q],"errors":[]}`, note, pinChild)
		case r.URL.Path == "/devices/combined/devices/v1":
			label := "Parent"
			if tenant != "parent" {
				label = "Child"
			}
			fmt.Fprintf(w, `{"meta":{"pagination":{"offset":0,"limit":500,"total":1}},"resources":[{"device_id":"dev-of-%s","hostname":"HostOf%s","status":"normal","notes":%q}],"errors":[]}`, label, label, note)
		default:
			fmt.Fprintf(w, `{"meta":{"pagination":{"offset":0,"limit":100,"total":0},"note":%q},"resources":[],"errors":[]}`, note)
		}
	}))
	t.Cleanup(f.srv.Close)
	return f
}

func (f *flightControl) state() (mints, reqs []string, minted map[string]string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	minted = map[string]string{}
	for k, v := range f.minted {
		minted[k] = v
	}
	return append([]string(nil), f.mints...), append([]string(nil), f.reqs...), minted
}

// fleetEnv isolates every user directory and puts the parent pair in the
// environment against f.
func fleetEnv(t *testing.T, f *flightControl) string {
	t.Helper()
	home := testenv.Isolate(t)
	t.Setenv("CROWDSTRIKE_HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	t.Setenv("XDG_DATA_HOME", filepath.Join(home, ".local", "share"))
	t.Setenv("XDG_STATE_HOME", filepath.Join(home, ".local", "state"))
	t.Setenv("XDG_CACHE_HOME", filepath.Join(home, ".cache"))
	for _, k := range []string{"CROWDSTRIKE_TOKEN_URL", "CROWDSTRIKE_OAUTH_SCOPE", "CROWDSTRIKE_CONFIG", "CROWDSTRIKE_NO_CONFIG_WRITE"} {
		t.Setenv(k, "")
	}
	t.Setenv("CROWDSTRIKE_BASE_URL", f.srv.URL)
	t.Setenv("FALCON_CLIENT_ID", pinParentID)
	t.Setenv("FALCON_CLIENT_SECRET", pinParentSecret)
	return home
}

// member-cid-token-pinned: every request fleet sync sends for the child CID
// carries only the member_cid token minted for it (never the parent's, never
// one from a second mint without member_cid), and the hosts stored under the
// child CID are the child tenant's. With --all-cids the parent token is
// minted first (to list the children), so the binding has a parent token in
// this process to swap in.
func TestFleetSyncChildReadsOnlyWithItsMemberCIDToken(t *testing.T) {
	for _, c := range []struct {
		name        string
		args        []string
		parentMints int
	}{
		{"--cids", []string{"--cids", pinChild}, 0},
		{"--member-cid", []string{"--member-cid", pinChild}, 0},
		{"--all-cids (parent token minted first)", []string{"--all-cids"}, 1},
	} {
		t.Run(c.name, func(t *testing.T) {
			f := newFlightControl(t, false)
			fleetEnv(t, f)
			db := filepath.Join(t.TempDir(), "fleet.db")
			out, errb, err := runFleet(t, append(append([]string{"fleet", "sync"}, c.args...), "--kinds", "hosts", "--db", db, "--json")...)
			if err != nil {
				t.Fatalf("fleet sync: %v\n%s%s", err, out, errb)
			}
			mints, reqs, minted := f.state()
			var childMints, parentMints int
			for _, cid := range mints {
				switch cid {
				case pinChild:
					childMints++
				case "":
					parentMints++
				default:
					t.Errorf("a token was minted for member_cid %q", cid)
				}
			}
			if childMints != 1 || parentMints != c.parentMints {
				t.Fatalf("want one member_cid mint and %d parent mint(s), got member_cid mints %d, parent mints %d (%v)", c.parentMints, childMints, parentMints, mints)
			}
			var hostReads int
			for _, r := range reqs {
				fields := strings.Fields(r)
				tok := fields[len(fields)-1]
				if fields[1] == "/mssp/queries/children/v1" {
					if minted[tok] != "parent" {
						t.Errorf("child discovery carried %q, want the parent token", tok)
					}
					continue
				}
				hostReads++
				if minted[tok] != pinChild {
					t.Errorf("%s %s carried the token of tenant %q, want only the member_cid token of %s", fields[0], fields[1], minted[tok], pinChild)
				}
			}
			if hostReads == 0 {
				t.Fatalf("fleet sync read no hosts; the test did not run:\n%s%s", out, errb)
			}
			ents, err := loadFleetEntities(context.Background(), db, kindHost)
			if err != nil {
				t.Fatal(err)
			}
			if len(ents) != 1 || ents[0].CID != pinChild || ents[0].ID != "dev-of-Child" {
				t.Fatalf("hosts stored: %+v, want only the child tenant's host under %s", ents, pinChild)
			}
		})
	}
}

// The member_cid token, the parent token, the client ID and the client secret
// never appear in stdout, stderr, the fleet store or any file fleet sync
// writes (the response cache included). The parent pair and both kinds of
// token are exercised: --all-cids lists the children with the parent token,
// fabric reads the Flight Control objects with it, hosts read the child with
// its member_cid token. An API answer that repeats a held credential is the
// credential kit's (its SQL driver and file sinks refuse it); in-process tests
// do not run the kit (credkit_seam_test.go), so the answers here repeat none.
func TestFleetSyncWritesNoCredential(t *testing.T) {
	f := newFlightControl(t, false)
	home := fleetEnv(t, f)
	db := filepath.Join(home, "fleet-scan.db")
	out, errb, err := runFleet(t, "fleet", "sync", "--all-cids", "--kinds", "hosts,fabric", "--db", db, "--json")
	if err != nil {
		t.Fatalf("fleet sync: %v\n%s%s", err, out, errb)
	}
	_, reqs, minted := f.state()
	var secrets []string
	for tok := range minted {
		secrets = append(secrets, tok)
	}
	var sawChild, sawParent bool
	for _, tenant := range minted {
		sawChild = sawChild || tenant == pinChild
		sawParent = sawParent || tenant == "parent"
	}
	if !sawChild || !sawParent || len(reqs) < 3 {
		t.Fatalf("want a parent and a member_cid token in use (minted %v, %d API requests); the scan would be vacuous", minted, len(reqs))
	}
	secrets = append(secrets, pinParentID, pinParentSecret)
	forms := func(s string) []string {
		return []string{s, base64.StdEncoding.EncodeToString([]byte(s)), base64.RawURLEncoding.EncodeToString([]byte(s)), url.QueryEscape(s)}
	}
	check := func(where, text string) {
		t.Helper()
		for _, s := range secrets {
			for _, f := range forms(s) {
				if strings.Contains(text, f) {
					t.Errorf("%s holds %q", where, f)
				}
			}
		}
	}
	check("stdout", out)
	check("stderr", errb)
	var scanned int
	var sawHost bool
	_ = filepath.WalkDir(home, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		b, rerr := os.ReadFile(path)
		if rerr != nil {
			return nil
		}
		scanned++
		sawHost = sawHost || bytes.Contains(b, []byte("HostOfChild"))
		rel, _ := filepath.Rel(home, path)
		check("file "+rel, string(b))
		return nil
	})
	if scanned == 0 || !sawHost {
		t.Fatalf("scanned %d files and none holds the child's synced host; the scan did not run", scanned)
	}
}
