// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

// Hand-written (NOT generated): Codex review round 1 of the crowdstrike 0.1.11
// branch, each class driven from its failing input against the built binary.
//
//   - token persistence loses client identity (handfix
//     minted-token-bound-to-its-client): `auth login` with an environment pair
//     saved that pair's token beside the stored pair, and the next run sent it
//     as the stored client's token.
//   - completed refresh never removes disappeared records (handfix
//     fleet-replace-only-a-proven-scope): a complete, unfiltered read of the
//     authenticated CID kept a remediated vulnerability, because that scope
//     was not bound to the client that read it.
//   - duplicate selections count the same entity repeatedly (handfix
//     fleet-selectors-deduplicated): a repeated kind, a kind alias or a case
//     variant, and a repeated CID, were each fetched and counted again.

package crowdstrikeblackbox

import (
	"database/sql"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	_ "modernc.org/sqlite"
)

// Fixture values: two API clients of two Falcon tenants on one cloud, the
// token each mints, and the member_cid token. No 8-character piece of any of
// them occurs in what the programs print on their own.
const (
	rfxIDA     = "aaidqqqqwwwweeeerrrr01"
	rfxSecretA = "aasecttttyyyyuuuuiiii02"
	rfxIDB     = "bbidppppoooollllkkkk03"
	rfxSecretB = "bbsecjjjjhhhhggggffff04"
	rfxTokA    = "aatokzzzzxxxxccccvvvv05"
	rfxTokB    = "bbtokbbbbnnnnmmmmllll06"
	rfxTokM    = "mmtokddddggggkkkkssss07"
	rfxHostA   = "hostoftenantone"
	rfxHostB   = "hostoftenanttwo"
	rfxChild   = "22222222222222222222222222abcdef"
	rfxEmpty   = `{"meta":{"pagination":{"total":0,"limit":500,"offset":0}},"resources":[],"errors":[]}`
)

func rfxVuln(id string) string {
	return `{"meta":{"pagination":{"total":1,"limit":500,"offset":0}},"resources":[{"id":"` + id +
		`","status":"open","cve":{"id":"CVE-2026-4242","severity":"CRITICAL"},"host_info":{"hostname":"hq-vuln-host"}}],"errors":[]}`
}

// rfxFalcon is a CrowdStrike cloud with two API clients in two tenants: the
// token endpoint (POST /oauth2/token, the pair in the form body) mints each
// client its own token, and the member_cid token for a child; every list
// answers by the token it was sent.
type rfxFalcon struct {
	url, caFile string
	mu          sync.Mutex
	vulns       map[string]string // bearer token -> Spotlight list body
	memberMints int
}

func newRfxFalcon(t *testing.T) *rfxFalcon {
	t.Helper()
	f := &rfxFalcon{vulns: map[string]string{}}
	srv := tlsServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/oauth2/token" {
			form, _ := url.ParseQuery(string(body))
			tok := ""
			switch {
			case form.Get("client_id") == rfxIDA && form.Get("client_secret") == rfxSecretA:
				tok = rfxTokA
			case form.Get("client_id") == rfxIDB && form.Get("client_secret") == rfxSecretB:
				tok = rfxTokB
			}
			if tok == "" {
				w.WriteHeader(http.StatusUnauthorized)
				_, _ = io.WriteString(w, `{"errors":[{"code":401,"message":"access denied, invalid client"}]}`)
				return
			}
			if m := form.Get("member_cid"); m != "" {
				f.mu.Lock()
				f.memberMints++
				f.mu.Unlock()
				tok = rfxTokM
			}
			fmt.Fprintf(w, `{"access_token":%q,"token_type":"bearer","expires_in":1799}`, tok)
			return
		}
		tok := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
		switch r.URL.Path {
		case "/devices/queries/devices/v1":
			host := map[string]string{rfxTokA: rfxHostA, rfxTokB: rfxHostB}[tok]
			if host == "" {
				w.WriteHeader(http.StatusUnauthorized)
				_, _ = io.WriteString(w, `{"errors":[{"code":401,"message":"access denied"}]}`)
				return
			}
			fmt.Fprintf(w, `{"meta":{"pagination":{"total":1,"limit":100,"offset":0}},"resources":[%q],"errors":[]}`, host)
		case "/spotlight/combined/vulnerabilities/v1":
			f.mu.Lock()
			b, ok := f.vulns[tok]
			f.mu.Unlock()
			if !ok {
				b = rfxEmpty
			}
			_, _ = io.WriteString(w, b)
		default:
			_, _ = io.WriteString(w, rfxEmpty)
		}
	}))
	f.url = srv.URL
	f.caFile = filepath.Join(t.TempDir(), "ca.pem")
	writeCA(t, srv.Certificate().Raw, f.caFile)
	return f
}

func writeCA(t *testing.T, der []byte, path string) {
	t.Helper()
	if err := os.WriteFile(path, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), 0o600); err != nil {
		t.Fatal(err)
	}
}

func (f *rfxFalcon) setVulns(tok, body string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.vulns[tok] = body
}

// rfxEnv is a clean environment on f, no credential in it, plus extra.
func rfxEnv(t *testing.T, f *rfxFalcon, extra ...string) (string, []string) {
	t.Helper()
	return isolated(t, append([]string{"CROWDSTRIKE_BASE_URL=" + f.url, "CROWDSTRIKE_CA_CERT=" + f.caFile}, extra...)...)
}

func pairEnv(env []string, id, secret string) []string {
	return append(append([]string{}, env...), "FALCON_CLIENT_ID="+id, "FALCON_CLIENT_SECRET="+secret)
}

// sumField adds every numeric member named name anywhere in the JSON text.
func sumField(t *testing.T, text, name string) (int, bool) {
	t.Helper()
	var v any
	if err := json.Unmarshal([]byte(text), &v); err != nil {
		t.Fatalf("output does not decode: %v\n%s", err, text)
	}
	total, found := 0, false
	var walk func(any)
	walk = func(x any) {
		switch x := x.(type) {
		case map[string]any:
			for k, c := range x {
				if n, ok := c.(float64); ok && k == name {
					total += int(n)
					found = true
					continue
				}
				walk(c)
			}
		case []any:
			for _, c := range x {
				walk(c)
			}
		}
	}
	walk(v)
	return total, found
}

// Class token persistence loses client identity: client A logs in with flags
// (its pair is stored); client B logs in from the environment; a run with no
// environment pair uses the stored pair A and must read tenant A, never with
// B's token. Every mint path that saves a token saves it only beside the
// client that minted it.
func TestR1FixLoginTokenStaysWithItsClient(t *testing.T) {
	t.Parallel()
	f := newRfxFalcon(t)
	home, env := rfxEnv(t, f)
	if out, errOut, code := runRaw(t, bbCLI, env, home, "", "auth", "login", "--client-id", rfxIDA, "--client-secret", rfxSecretA); code != 0 {
		t.Fatalf("control: auth login for client A failed (%d):\n%s\n%s", code, out, errOut)
	}
	out2, errOut2, code2 := runRaw(t, bbCLI, pairEnv(env, rfxIDB, rfxSecretB), home, "", "auth", "login")
	out, errOut, code := runRaw(t, bbCLI, env, home, "", "devices", "query-by-filter", "--json")
	if strings.Contains(out+errOut, rfxHostB) {
		t.Fatalf("a run with the stored client A read tenant B (B's token was saved beside A's pair):\nlogin B (%d): %s %s\nread: %s %s", code2, out2, errOut2, out, errOut)
	}
	if code != 0 || !strings.Contains(out, rfxHostA) {
		t.Fatalf("control: the stored client A must read tenant A (%d):\n%s\n%s", code, out, errOut)
	}
	// The stored token is A's: the store never holds B's token beside A's pair,
	// and the login that could not save says so (it does not report a save).
	store := filepath.Join(home, "data", "credentials.toml")
	if b, err := os.ReadFile(store); err == nil && strings.Contains(string(b), rfxTokB) {
		t.Errorf("the credential store holds client B's token beside client A's pair")
	}
	if code2 == 0 || !strings.Contains(errOut2, "another client ID") {
		t.Errorf("auth login for client B over stored client A must refuse the save and say why (%d):\n%s\n%s", code2, out2, errOut2)
	}
	// An environment pair with nothing stored still logs in (control): its
	// token is saved with no client beside it, and a run with no pair uses it.
	home2, env2 := rfxEnv(t, f)
	if out, errOut, code := runRaw(t, bbCLI, pairEnv(env2, rfxIDB, rfxSecretB), home2, "", "auth", "login"); code != 0 {
		t.Fatalf("control: an environment login with nothing stored must succeed (%d):\n%s\n%s", code, out, errOut)
	}
	if out, errOut, code := runRaw(t, bbCLI, env2, home2, "", "devices", "query-by-filter", "--json"); code != 0 || !strings.Contains(out, rfxHostB) {
		t.Fatalf("control: the token client B logged in with must read tenant B (%d):\n%s\n%s", code, out, errOut)
	}
}

// Class completed refresh never removes disappeared records: a complete,
// unfiltered read of the authenticated CID replaces the rows that client read
// before (a remediated vulnerability leaves fleet vulns and the scorecard),
// and never another client's rows in a shared --db.
func TestR1FixSelfRefreshRemovesRemediated(t *testing.T) {
	t.Parallel()
	f := newRfxFalcon(t)
	home, env := rfxEnv(t, f)
	db := filepath.Join(home, "self.db")
	envA, envB := pairEnv(env, rfxIDA, rfxSecretA), pairEnv(env, rfxIDB, rfxSecretB)
	sync := func(env []string) {
		t.Helper()
		if out, errOut, code := runRaw(t, bbCLI, env, home, "", "fleet", "sync", "--kinds", "vulns", "--db", db, "--json"); code != 0 {
			t.Fatalf("fleet sync (%d):\n%s\n%s", code, out, errOut)
		}
	}
	crit := func() (int, string) {
		t.Helper()
		out, errOut, code := runRaw(t, bbCLI, env, home, "", "fleet", "scorecard", "--db", db, "--json")
		if code != 0 {
			t.Fatalf("fleet scorecard (%d):\n%s\n%s", code, out, errOut)
		}
		n, _ := sumField(t, out, "critical_vulns")
		return n, out
	}
	f.setVulns(rfxTokA, rfxVuln("vulofclientone"))
	f.setVulns(rfxTokB, rfxVuln("vulofclienttwo"))
	sync(envA)
	if n, out := crit(); n != 1 {
		t.Fatalf("control: the first sync must store client A's critical vulnerability (the write was kept), got %d:\n%s", n, out)
	}
	sync(envB)
	if n, out := crit(); n != 2 {
		t.Fatalf("control: client B's sync must add its own critical vulnerability, got %d:\n%s", n, out)
	}
	// Client A remediates: its complete, unfiltered list is empty.
	f.setVulns(rfxTokA, rfxEmpty)
	sync(envA)
	n, out := crit()
	vulns, _, _ := runRaw(t, bbCLI, env, home, "", "fleet", "vulns", "--db", db, "--json")
	if strings.Contains(vulns, "vulofclientone") || n != 1 {
		t.Errorf("a complete refresh by client A kept its remediated vulnerability (scorecard critical_vulns %d, want 1):\n%s\n%s", n, out, vulns)
	}
	if !strings.Contains(vulns, "vulofclienttwo") {
		t.Errorf("client A's refresh deleted client B's vulnerability in the shared --db:\n%s", vulns)
	}
	// A filtered read still replaces nothing (fleet-replace-only-a-proven-scope).
	f.setVulns(rfxTokB, rfxEmpty)
	if o, e, code := runRaw(t, bbCLI, envB, home, "", "fleet", "sync", "--kinds", "vulns", "--vuln-filter", "status:'open'+aid:'aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa'", "--db", db, "--json"); code != 0 {
		t.Fatalf("filtered fleet sync (%d):\n%s\n%s", code, o, e)
	}
	if vulns, _, _ := runRaw(t, bbCLI, env, home, "", "fleet", "vulns", "--db", db, "--json"); !strings.Contains(vulns, "vulofclienttwo") {
		t.Errorf("a filtered read deleted client B's vulnerability:\n%s", vulns)
	}
	// The store holds no credential (the scope's client binding is a one-way
	// digest, not the client ID).
	b, err := os.ReadFile(db)
	if err != nil {
		t.Fatal(err)
	}
	for _, v := range []string{rfxIDA, rfxSecretA, rfxIDB, rfxSecretB, rfxTokA, rfxTokB} {
		if strings.Contains(string(b), v) {
			t.Errorf("the fleet store holds a credential")
		}
	}
}

// Class duplicate selections count the same entity repeatedly: a kind named
// twice, a kind alias, a case variant, and a CID named twice (or in another
// case) are each read once, so one open critical vulnerability is one in the
// posture snapshot, and fleet trend calls an unchanged tenant flat.
func TestR1FixRepeatedSelectorsCountOnce(t *testing.T) {
	t.Parallel()
	f := newRfxFalcon(t)
	home, env := rfxEnv(t, f)
	env = pairEnv(env, rfxIDA, rfxSecretA)
	db := filepath.Join(home, "trend.db")
	f.setVulns(rfxTokM, rfxVuln("vulofthechild"))
	run := func(args ...string) string {
		t.Helper()
		out, errOut, code := runRaw(t, bbCLI, env, home, "", append(append([]string{"fleet", "sync"}, args...), "--db", db, "--json")...)
		if code != 0 {
			t.Fatalf("fleet sync %v (%d):\n%s\n%s", args, code, out, errOut)
		}
		return out
	}
	run("--member-cid", rfxChild, "--kinds", "hosts,alerts,vulns")
	for _, args := range [][]string{
		{"--member-cid", rfxChild, "--kinds", "hosts,alerts,vulns,vulns"},
		{"--member-cid", rfxChild, "--kinds", "hosts,alerts,vulns,spotlight"},
		{"--member-cid", rfxChild, "--kinds", "HOSTS,alerts,Vulns,vulns"},
		{"--cids", rfxChild + "," + rfxChild, "--kinds", "hosts,alerts,vulns"},
		{"--cids", rfxChild + ", " + strings.ToUpper(rfxChild), "--kinds", "hosts,alerts,vulns"},
	} {
		f.mu.Lock()
		f.memberMints = 0
		f.mu.Unlock()
		sum := run(args...)
		if n, _ := sumField(t, sum, "entities"); n != 1 {
			t.Errorf("%v: fleet sync counted %d entities, want 1:\n%s", args, n, sum)
		}
		f.mu.Lock()
		mints := f.memberMints
		f.mu.Unlock()
		if mints != 1 {
			t.Errorf("%v: fleet sync minted %d member_cid tokens, want 1 (the CID was read more than once)", args, mints)
		}
		out, errOut, code := runRaw(t, bbCLI, env, home, "", "fleet", "trend", "--db", db, "--json")
		if code != 0 {
			t.Fatalf("fleet trend (%d):\n%s\n%s", code, out, errOut)
		}
		crit, _ := sumField(t, out, "critical_vulns")
		delta, _ := sumField(t, out, "critical_vulns_delta")
		if crit != 1 || delta != 0 || strings.Contains(out, `"worse"`) {
			t.Errorf("%v: fleet trend reports critical_vulns %d (delta %d), want 1 (delta 0) for one unchanged vulnerability:\n%s", args, crit, delta, out)
		}
	}
}

// Class incomplete results lose their qualification across the CLI-to-MCP
// handoff (Codex round 3 of the 0.1.11 branch, handfix
// fleet-legacy-rows-not-current): a rollup over the MCP server excludes a
// pre-upgrade row and must say so; the server forwards only hint: and warning:
// lines of a tool's stderr.
func TestR3LegacyRowLineReachesMCP(t *testing.T) {
	t.Parallel()
	f := newRfxFalcon(t)
	home, env := rfxEnv(t, f)
	envA := pairEnv(env, rfxIDA, rfxSecretA)
	f.setVulns(rfxTokA, rfxVuln("vulofclientone"))
	if out, errOut, code := runRaw(t, bbCLI, envA, home, "", "fleet", "sync", "--kinds", "vulns", "--json"); code != 0 {
		t.Fatalf("fleet sync (%d):\n%s\n%s", code, out, errOut)
	}
	// The default store, its rows made as a version before the owner column left them.
	made := 0
	_ = filepath.WalkDir(home, func(p string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(p, ".db") {
			return nil
		}
		db, oerr := sql.Open("sqlite", p)
		if oerr != nil {
			t.Fatal(oerr)
		}
		defer db.Close()
		if res, xerr := db.Exec(`UPDATE fleet_entities SET owner = ''`); xerr == nil {
			n, _ := res.RowsAffected()
			made += int(n)
		}
		return nil
	})
	if made != 1 {
		t.Fatalf("control: one stored row expected, made %d", made)
	}
	e := &bbEnv{env: envA, home: home}
	replies := mcpReplies(e.mcpCalls(t, []map[string]any{
		{"name": "fleet_vulns", "arguments": map[string]any{}},
		{"name": "fleet_scorecard", "arguments": map[string]any{}},
	}))
	for id, tool := range map[int]string{1: "fleet_vulns", 2: "fleet_scorecard"} {
		if !strings.Contains(replies[id], "1 row(s) written before this version") {
			t.Errorf("%s dropped a row without saying so over MCP:\n%s", tool, replies[id])
		}
		if strings.Contains(replies[id], "vulofclientone") {
			t.Errorf("%s reports the pre-upgrade row as current:\n%s", tool, replies[id])
		}
	}
}
