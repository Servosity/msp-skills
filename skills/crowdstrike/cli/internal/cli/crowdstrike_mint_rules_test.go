// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

// The CrowdStrike token rules on every mint path the CLI has: the automatic
// mint a read triggers (the generated client), `auth login`, and the fleet
// member_cid mint.
//   - per-cloud-token-url: with only CROWDSTRIKE_BASE_URL set (the fake stands
//     for a Falcon cloud other than US-1), every mint posts to that cloud's own
//     /oauth2/token; a base URL carrying userinfo names no token URL, so no
//     path posts anything (crowdstrikeblackbox TestEveryMintStaysOnItsCloud
//     proves on the built binary, through a recording proxy, that nothing goes
//     to api.crowdstrike.com either).
//   - token-request-body-credentials: the token POST carries client_id and
//     client_secret in an application/x-www-form-urlencoded body
//     (grant_type=client_credentials, member_cid for a child CID) and no HTTP
//     Basic header (4.34.2 rendered Basic, which CrowdStrike does not document).

import (
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"testing"
)

func TestEveryMintPostsTheFormToItsCloud(t *testing.T) {
	const id, secret, access = "everyMintClientId-4f2a", "everyMintClientSecret-9c3e1b", "everyMintAccess-7d1e"
	for _, c := range []struct {
		name      string
		args      []string
		memberCID string
	}{
		{"automatic mint (a read)", []string{"devices", "query-by-filter", "--json"}, ""},
		{"auth login", []string{"auth", "login"}, ""},
		{"fleet sync member_cid mint", []string{"fleet", "sync", "--cids", "cid-every-mint", "--kinds", "hosts", "--json"}, "cid-every-mint"},
	} {
		t.Run(c.name, func(t *testing.T) {
			credRulesHome(t)
			m := newFakeFalcon(t, id, secret, access, func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(falconList()))
			})
			t.Setenv("CROWDSTRIKE_BASE_URL", m.srv.URL)
			t.Setenv("FALCON_CLIENT_ID", id)
			t.Setenv("FALCON_CLIENT_SECRET", secret)
			args := append([]string{}, c.args...)
			if args[0] == "fleet" {
				args = append(args, "--db", t.TempDir()+"/fleet.db")
			}
			out, errb, err := runFleet(t, args...)
			if err != nil {
				t.Fatalf("%v: %v\n%s%s", args, err, out, errb)
			}
			if m.mints.Load() != 1 || m.bad.Load() != 0 {
				t.Fatalf("want one accepted mint at %s/oauth2/token, got %d (refused %d; last form %v, Authorization %q)", m.srv.URL, m.mints.Load(), m.bad.Load(), m.lastMintForm.Load(), m.lastMintAuth.Load())
			}
			ct, raw, _ := strings.Cut(fmt.Sprint(m.lastMintForm.Load()), " ")
			form, _ := url.ParseQuery(raw)
			if ct != "application/x-www-form-urlencoded" || form.Get("grant_type") != "client_credentials" || form.Get("client_id") != id ||
				form.Get("client_secret") != secret || form.Get("member_cid") != c.memberCID || m.lastMintAuth.Load() != "" {
				t.Fatalf("token request: Content-Type %q, form %q, Authorization %q; want the pair in the form body (member_cid %q) and no Basic header",
					ct, raw, m.lastMintAuth.Load(), c.memberCID)
			}
		})
	}
}

// A base URL carrying userinfo cannot name a host the client secret may be
// posted to: no mint path sends anything, each says why.
func TestUnsafeBaseURLPostsNothing(t *testing.T) {
	const id, secret = "unsafeBaseClientId-2a6c", "unsafeBaseClientSecret-8e4d1f"
	var hits int
	var mu sync.Mutex
	m := newFakeFalcon(t, id, secret, "unsafeBaseAccess-1", func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		hits++
		mu.Unlock()
		_, _ = w.Write([]byte(falconList()))
	})
	u, _ := url.Parse(m.srv.URL)
	unsafe := "http://ro:unsafePw-5b2e@" + u.Host
	for _, args := range [][]string{
		{"devices", "query-by-filter", "--json"},
		{"auth", "login"},
		{"fleet", "sync", "--cids", "cid-unsafe", "--kinds", "hosts", "--json"},
	} {
		credRulesHome(t)
		t.Setenv("CROWDSTRIKE_BASE_URL", unsafe)
		t.Setenv("FALCON_CLIENT_ID", id)
		t.Setenv("FALCON_CLIENT_SECRET", secret)
		if args[0] == "fleet" {
			args = append(args, "--db", t.TempDir()+"/fleet.db")
		}
		out, errb, err := runFleet(t, args...)
		if err == nil && !strings.Contains(out+errb, "token URL") {
			t.Errorf("%v with a base URL carrying userinfo ran without refusing:\n%s%s", args, out, errb)
		}
		if strings.Contains(out+errb, secret) || strings.Contains(out+errb, "unsafePw-5b2e") {
			t.Errorf("%v printed a credential:\n%s%s", args, out, errb)
		}
	}
	mu.Lock()
	defer mu.Unlock()
	if n := m.mints.Load() + m.bad.Load(); n != 0 || hits != 0 {
		t.Fatalf("with an unsafe base URL the fake received %d token request(s) and %d API request(s)", n, hits)
	}
}
