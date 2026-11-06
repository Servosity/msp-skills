// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package vsphereblackbox

// The credential classes the previous publish review left open, run on the built
// binaries with its own inputs (each seen failing on the code before the kit:
// .review/seen in the publish worktree), plus what adopting the kit decided:
// run profiles, the MCP child's credentials, the plain http refusal and the size
// of what the kit can judge.

import (
	"encoding/csv"
	"encoding/pem"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
)

// filesUnder returns every file under dir but the named credential files, joined.
func filesUnder(t *testing.T, dir string, skip ...string) string {
	t.Helper()
	var b strings.Builder
	_ = filepath.Walk(dir, func(p string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return nil
		}
		for _, s := range append(skip, "credentials.toml") {
			if filepath.Base(p) == s {
				return nil
			}
		}
		data, _ := os.ReadFile(p)
		b.WriteString(p + "\n" + string(data) + "\n")
		return nil
	})
	return b.String()
}

// undecoded-encoding-gap: ASCII85 of base64 of the password.
func TestASCII85OfBase64NeverWritten(t *testing.T) {
	t.Parallel()
	const enc = "<CK6#@l%V=@r5(+" // ASCII85 of "UEBzc3cwcmQ=", base64 of P@ssw0rd
	home := t.TempDir()
	out := runWith(t, cleanEnv(home, "VSPHERE_HOME="+home, "VSPHERE_PASSWORD=P@ssw0rd"), "feedback", enc, "--json")
	for where, text := range map[string]string{"output": out, "files": filesUnder(t, home)} {
		if strings.Contains(text, enc) || strings.Contains(text, "UEBzc3cwcmQ") {
			t.Errorf("%s carry the encoded password:\n%s", where, text)
		}
	}
}

// outbound-sink-bypasses-chokepoint, same encoding: the --deliver webhook URL
// carried the recoverable password to another host.
func TestEncodedPasswordNeverDelivered(t *testing.T) {
	t.Parallel()
	var mu sync.Mutex
	var seen strings.Builder
	sink := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		seen.WriteString(r.RequestURI + "\n")
		mu.Unlock()
		w.WriteHeader(http.StatusNoContent)
	}))
	t.Cleanup(sink.Close)
	home := t.TempDir()
	env := cleanEnv(home, "VSPHERE_HOME="+home, "VSPHERE_BASE_URL=https://vc.example", "VSPHERE_PASSWORD=P@ssw0rd")
	runWith(t, env, "version", "--deliver", "webhook:"+sink.URL+"/receive?note=%3CCK6%23%40l%25V%3D%40r5%28%2B")
	runWith(t, env, "version", "--deliver", "webhook:"+sink.URL+"/control")
	mu.Lock()
	got := seen.String()
	mu.Unlock()
	if strings.Contains(got, "CK6") {
		t.Errorf("the sink received the encoded password:\n%s", got)
	}
	if !strings.Contains(got, "/control") {
		t.Errorf("a clean webhook received nothing; the test did not run:\n%s", got)
	}
}

// file-sink-bypasses-chokepoint: with the .agentcookie-managed marker, auth
// set-credentials keeps the password in config.toml. That file is then a declared
// credential file: 0600, and no other file or output repeats the password, in that
// run or the next.
func TestAgentcookieConfigIsADeclaredCredentialFile(t *testing.T) {
	t.Parallel()
	const pw = "AgentCookiePw-Q7n9"
	home := t.TempDir()
	if err := os.MkdirAll(filepath.Join(home, "config"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(home, "config", ".agentcookie-managed"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	env := cleanEnv(home, "VSPHERE_HOME="+home, "VSPHERE_BASE_URL=https://vc.example")
	out := runWith(t, env, "auth", "set-credentials", "readonly@vsphere.local", pw)
	out += runWith(t, env, "feedback", "the password is "+pw, "--json")
	out += runWith(t, env, "feedback", "an ordinary note", "--json")
	if !strings.Contains(filesUnder(t, home), "an ordinary note") {
		t.Fatalf("the ordinary feedback wrote nothing; the test did not run:\n%s", out)
	}
	for where, text := range map[string]string{"output": out, "files": filesUnder(t, home, "config.toml")} {
		if strings.Contains(text, pw) {
			t.Errorf("%s carry the password:\n%s", where, text)
		}
	}
	if runtime.GOOS != "windows" {
		if info, err := os.Stat(filepath.Join(home, "config", "config.toml")); err == nil && info.Mode().Perm() != 0o600 {
			t.Errorf("config.toml mode %v, want 0600", info.Mode().Perm())
		}
	}
}

// learn-journal: the journal and the learn tables are sinks too. A credential as
// a positional, as a flag value or in a failed command stays out of them.
func TestLearnJournalNeverKeepsACredential(t *testing.T) {
	t.Parallel()
	e := newEnv(t, quietServer(t), true)
	for _, args := range [][]string{
		{"vm", "get", bbPassword, "--json"},
		{"vm", "list", "--select", bbPassword, "--json"},
		{"vm", "list", "--nosuchflag=" + bbPassword},
		{"vm", "list", "--json"},
	} {
		e.run(t, args...)
	}
	all := filesUnder(t, e.home)
	if !strings.Contains(all, "journal-") {
		t.Fatalf("no journal was written; the test did not run:\n%.1000s", all)
	}
	assertNoEcho(t, "the home directory", all)
}

// credkit-profiles-held: a run-profile store is read after the kit started,
// so startup holds what it carries as a source: a value under a flag a profile
// does not apply, every duplicate of it, and what a base-url value carries. A
// saved --select stays usable, and so does a URL in an allowed flag such as
// --deliver (free text is no source, contract 1). A store that does not parse
// is read by the kit's HoldBody.
func TestRunProfilesAreHeldAtStartup(t *testing.T) {
	t.Parallel()
	const (
		unknown = "UnknownFlagValue-M2"
		first   = "FirstDuplicate-R8"
		cred    = "SavedPasswordFlag-W3"
		urlPw   = "BaseUrlPw-H6"
		broken  = "BrokenStoreValue-T5"
	)
	home := t.TempDir()
	cfg := filepath.Join(home, "config")
	if err := os.MkdirAll(cfg, 0o700); err != nil {
		t.Fatal(err)
	}
	store := filepath.Join(cfg, "profiles.json")
	// nosuchflag twice: json.Unmarshal keeps only the second.
	body := fmt.Sprintf(`{"profiles":{"p":{"name":"p","values":{"select":"name,power_state","deliver":"webhook:https://h.example/hook","nosuchflag":%q,"nosuchflag":%q,"password":%q}}}}`, first, unknown, cred)
	if err := os.WriteFile(store, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	env := cleanEnv(home, "VSPHERE_HOME="+home)
	var out strings.Builder
	for _, v := range []string{unknown, first, cred} {
		out.WriteString(runWith(t, env, "feedback", "repeats "+v, "--json"))
	}
	out.WriteString(runWith(t, env, "feedback", "the saved select name,power_state and webhook:https://h.example/hook", "--json"))
	written := filesUnder(t, home, "profiles.json")
	if !strings.Contains(written, "name,power_state") || !strings.Contains(written, "h.example/hook") {
		t.Errorf("an allowed profile value was held:\n%s\n%s", out.String(), written)
	}
	// This CLI has no base-url flag, so a base-url value is held whole.
	body = fmt.Sprintf(`{"profiles":{"p":{"name":"p","values":{"base-url":"https://u:%s@vc.example"}}}}`, urlPw)
	if err := os.WriteFile(store, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	out.WriteString(runWith(t, env, "feedback", "repeats "+urlPw, "--json"))
	written += filesUnder(t, home, "profiles.json")
	for _, v := range []string{unknown, first, cred, urlPw} {
		if strings.Contains(out.String(), v) || strings.Contains(written, v) {
			t.Errorf("%s was not held:\n%s\n%s", v, out.String(), written)
		}
	}
	// A store that does not parse has no names to tell apart.
	if err := os.WriteFile(store, []byte(`{"profiles":{"p":{"values":{"select":"`+broken+`"`), 0o600); err != nil {
		t.Fatal(err)
	}
	got := runWith(t, env, "feedback", "repeats "+broken, "--json")
	if strings.Contains(got, broken) || strings.Contains(filesUnder(t, home, "profiles.json"), broken) {
		t.Errorf("a value in a broken store was not held:\n%s", got)
	}
}

// The kit refuses a credential over plain http, loopback included; no request
// leaves with it.
func TestPlainHTTPNeverCarriesTheCredential(t *testing.T) {
	t.Parallel()
	var hits atomic.Int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, _, ok := r.BasicAuth(); ok {
			hits.Add(1)
		}
		w.WriteHeader(http.StatusUnauthorized)
	}))
	t.Cleanup(srv.Close)
	home := t.TempDir()
	out := runWith(t, cleanEnv(home, "VSPHERE_BASE_URL="+srv.URL, "VSPHERE_USERNAME="+bbUser, "VSPHERE_PASSWORD="+bbPassword), "vm", "list", "--json")
	if hits.Load() != 0 {
		t.Errorf("%d requests carried the credential over plain http:\n%s", hits.Load(), out)
	}
	assertNoEcho(t, "the refusal", out)
}

// The SSO user name is an identifier: before a login nothing withholds it.
func TestUserNameIsNotWithheldBeforeALogin(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	env := cleanEnv(home, "VSPHERE_HOME="+home, "VSPHERE_USERNAME="+bbUser, "VSPHERE_PASSWORD="+bbPassword)
	if out := runWith(t, env, "feedback", "the account "+bbUser, "--json"); strings.Contains(out, withheldNotice) || !strings.Contains(filesUnder(t, home), bbUser) {
		t.Errorf("the user name was withheld:\n%s", out)
	}
}

// A fleet-sized inventory stays inside what the kit can judge: printed whole, and
// written row by row, not withheld on budget.
//
// Not parallel, which only lowers the odds: on a loaded machine this output can
// lose its tail today. A 10 ms pause splits the document, the kit's stream
// context cannot hold it past 16 KiB, and the rest is dropped with exit 74. That
// is a kit bug, not this connector's, and a failure here at 65536 bytes is it.
func TestLargeInventoryIsNotWithheldOnBudget(t *testing.T) {
	const printed, exported = 3000, 150
	body := func(n int) string {
		var list strings.Builder
		list.WriteString("[")
		for i := range n {
			if i > 0 {
				list.WriteString(",")
			}
			fmt.Fprintf(&list, `{"vm":"vm-%d","name":"app-%04d.corp.example","power_state":"POWERED_ON","cpu_count":%d,"memory_size_MiB":%d}`, i, i, i%16+1, (i%64+1)*1024)
		}
		return list.String() + "]"
	}
	var count atomic.Int64
	count.Store(printed)
	srv := tlsServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/api/session":
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`"large-inventory-session-7e1a"`))
		case r.URL.Path == "/api/vcenter/vm":
			_, _ = w.Write([]byte(body(int(count.Load()))))
		case strings.HasPrefix(r.URL.Path, "/api/vcenter/"):
			_, _ = w.Write([]byte(`[]`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	ca := filepath.Join(t.TempDir(), "ca.pem")
	if err := os.WriteFile(ca, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: srv.Certificate().Raw}), 0o600); err != nil {
		t.Fatal(err)
	}
	home := t.TempDir()
	env := cleanEnv(home, "VSPHERE_BASE_URL="+srv.URL, "VSPHERE_CA_CERT="+ca, "VSPHERE_USERNAME="+bbUser, "VSPHERE_PASSWORD="+bbPassword)
	got := runWith(t, env, "vm", "list", "--json", "--data-source", "live", "--rate-limit", "0")
	if strings.Contains(got, withheldNotice) || !strings.Contains(got, fmt.Sprintf("app-%04d", printed-1)) {
		t.Errorf("vm list --json lost the inventory: withheld=%v, %d bytes, ends:\n%s", strings.Contains(got, withheldNotice), len(got), got[max(0, len(got)-600):])
	}
	count.Store(exported)
	dir := filepath.Join(home, "rv")
	got = runWith(t, env, "rvtools", "export", "--out", dir, "--rate-limit", "0")
	f, err := os.Open(filepath.Join(dir, "RVTools_tabvInfo.csv"))
	if err != nil {
		t.Fatalf("no vInfo tab: %v\n%.800s", err, got)
	}
	defer f.Close()
	rows, err := csv.NewReader(f).ReadAll()
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != exported+1 {
		t.Errorf("vInfo has %d rows, want %d", len(rows), exported+1)
	}
	for _, row := range rows {
		if strings.Contains(strings.Join(row, ","), withheldNotice) {
			t.Fatalf("a row was withheld: %v", row)
		}
	}
}

// Class credential-in-url-setting: a URL the program reads from its environment
// can carry a credential of its own, held at startup, before a parse error or a
// send repeats it. A setting with no credential prints as usual, and an MCP CLI
// child still starts with the setting in place.
func TestEnvironmentURLCredentialsAreHeld(t *testing.T) {
	t.Parallel()
	const secret = "FeedbackSecret9z"
	for name, endpoint := range map[string]string{
		"query":             "https://feedback.example:bad/?token=" + secret,
		"userinfo password": "https://reporter:" + secret + "@feedback.example:bad/",
		"percent-encoded":   "https://feedback.example:bad/?token=FeedbackSecret%399z",
		"bad escape":        "https://reporter:" + secret + "%zz@127.0.0.1:1/fb",
	} {
		t.Run(name, func(t *testing.T) {
			e := newEnv(t, quietServer(t), true)
			e.env = append(e.env, "VSPHERE_FEEDBACK_ENDPOINT="+endpoint)
			out := e.run(t, "feedback", "note", "--send", "--json")
			if strings.Contains(out, secret) || strings.Contains(out, "FeedbackSecret%39") {
				t.Fatalf("the feedback URL's credential was printed:\n%.1500s", out)
			}
		})
	}
	t.Run("a URL with no credential still prints", func(t *testing.T) {
		e := newEnv(t, quietServer(t), true)
		e.env = append(e.env, "VSPHERE_FEEDBACK_ENDPOINT=https://feedback.example:bad/")
		if out := e.run(t, "feedback", "note", "--send", "--json"); !strings.Contains(out, "feedback.example") || strings.Contains(out, withheldNotice) {
			t.Fatalf("want the URL in the error, got:\n%.1500s", out)
		}
	})
	t.Run("an MCP CLI child still starts", func(t *testing.T) {
		e := newEnv(t, quietServer(t), true)
		e.env = append(e.env, "VSPHERE_FEEDBACK_ENDPOINT=https://feedback.example/?token="+secret)
		out := e.mcpCalls(t, []map[string]any{{"name": "migration_blockers", "arguments": map[string]any{}}})
		if !strings.Contains(out, "HQ-Record") {
			t.Fatalf("the child did not answer:\n%.2000s", out)
		}
	})
}
