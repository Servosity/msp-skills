// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package exoblackbox

// The classes the adoption closes, each from its failing input, through the built
// binaries. Each test was seen failing first, on the tree before the kit or with the
// fix reverted.

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// listingAPI answers every cmdlet with one ordinary record and counts the writes.
func listingAPI(t *testing.T) (*bbEnv, *atomic.Int32) {
	t.Helper()
	var writes atomic.Int32
	srv := tlsServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		if !strings.HasPrefix(cmdletOf(body), "Get-") {
			writes.Add(1)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"value":[{"Identity":"rosa@example.com","Name":"Rosa Diaz","RecipientTypeDetails":"UserMailbox","HiddenFromAddressListsEnabled":false}]}`))
	}))
	return newEnv(t, &server{url: srv.URL, caFile: pemFile(t, srv)}, true), &writes
}

// noPiece fails on the value or any 8-character piece of it.
func noPiece(t *testing.T, where, text, value string) {
	t.Helper()
	for i := 0; i+8 <= len(value); i++ {
		if strings.Contains(text, value[i:i+8]) {
			t.Errorf("%s shows a piece of the credential (%q):\n%.1200s", where, value[i:i+8], text)
			return
		}
	}
}

// Class outbound-sink-bypasses-chokepoint. A held token in the URL or a header of the
// feedback endpoint or a --deliver webhook is refused before anything is sent, neither
// sender follows a redirect, and a credential the endpoint variable itself carries is
// held. A clean send still arrives, so a refusal is not a broken sender.
func TestOutboundSendersRefuseAHeldTokenAndFollowNoRedirect(t *testing.T) {
	var offHost atomic.Int32
	off := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { offHost.Add(1) }))
	t.Cleanup(off.Close)
	var mu sync.Mutex
	var reached []string
	sink := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		reached = append(reached, r.URL.Path)
		mu.Unlock()
		if r.URL.Path == "/moved" {
			http.Redirect(w, r, off.URL+"/taken", http.StatusFound)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	t.Cleanup(sink.Close)
	take := func() []string {
		mu.Lock()
		defer mu.Unlock()
		got := reached
		reached = nil
		return got
	}
	e, _ := listingAPI(t)
	feedback := func(endpoint string, more ...string) string {
		t.Helper()
		return e.with(append([]string{"EXCHANGE_ONLINE_FEEDBACK_ENDPOINT=" + endpoint}, more...)...).run(t, "feedback", "a note", "--send")
	}
	deliver := func(url string, more ...string) string {
		t.Helper()
		return e.with(more...).run(t, "mailboxes", "list", bbTenant, "--json", "--deliver", "webhook:"+url)
	}
	for _, url := range []string{
		sink.URL + "/collect?token=" + bbToken,
		sink.URL + "/hooks/" + bbToken,
		strings.Replace(sink.URL, "http://", "http://u:"+bbToken+"@", 1) + "/collect",
	} {
		noPiece(t, "feedback output", feedback(url), bbToken)
		noPiece(t, "deliver output", deliver(url), bbToken)
	}
	// A header: the operator's User-Agent is what the deliver sender sends.
	noPiece(t, "deliver output", deliver(sink.URL+"/header", "EXCHANGE_ONLINE_USER_AGENT="+bbToken), bbToken)
	if got := take(); len(got) != 0 {
		t.Errorf("a request carrying a held token reached the sink: %q", got)
	}
	feedback(sink.URL + "/moved")
	deliver(sink.URL + "/moved")
	if got := take(); len(got) != 2 {
		t.Errorf("want both senders to reach the redirecting sink once, got %q", got)
	}
	if offHost.Load() != 0 {
		t.Errorf("a sender followed a redirect off the host it was given (%d)", offHost.Load())
	}
	feedback(sink.URL + "/collect")
	deliver(sink.URL + "/deliver")
	if got := strings.Join(take(), " "); got != "/collect /deliver" {
		t.Errorf("a clean send did not arrive: %q", got)
	}
	// A credential only the endpoint variable carries: a refused connection names the URL.
	closed := httptest.NewServer(http.NotFoundHandler())
	dead := closed.URL
	closed.Close()
	const urlOnly = "FeedbackUrlSecret7Qx2Lm"
	noPiece(t, "a failed feedback send", feedback(dead+"/collect?token="+urlOnly), urlOnly)
}

// Class startup-output-before-the-filter: a credential in a malformed or legacy config
// file never appears in the startup output of either binary, and neither crashes on it.
// A file the kit cannot read precisely has every word held, so one whose word is public
// text refuses to start with status 78 (contract section 2): a refusal, not a crash.
func TestStartupMalformedOrLegacyConfig(t *testing.T) {
	const secret = "CfgFileSecretQ7x2Lm9Vp4"
	files := map[string]string{
		"unterminated": "auth_header = \"Bearer " + secret + "\n",
		"not toml":     secret + " is not a config\n",
		"json":         `{"auth_header":"Bearer ` + secret + `"}`,
		"bad section":  "[profile\ntoken = \"" + secret + "\"\n",
		"legacy key":   "exchange_online_token = \"" + secret + "\"\n",
		"unknown key":  "api_key = \"" + secret + "\"\n",
		"wrong type":   "auth_header = [\"" + secret + "\"]\n",
		"url password": "base_url = \"https://u:" + secret + "@127.0.0.1:1/\"\n",
		"url query":    "base_url = \"https://127.0.0.1:1/?key=" + secret + "\"\n",
	}
	crash := regexp.MustCompile(`panic:|goroutine \d+`)
	for name, content := range files {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			e, _ := listingAPI(t)
			named := filepath.Join(e.home, "named.toml")
			legacyDir := filepath.Join(e.home, ".config", "exchange-online-cli")
			if err := os.MkdirAll(legacyDir, 0o700); err != nil {
				t.Fatal(err)
			}
			for _, f := range []string{named, filepath.Join(legacyDir, "config.toml")} {
				if err := os.WriteFile(f, []byte(content), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			moved := e.with("XDG_CONFIG_HOME=" + filepath.Join(e.home, "xdg")) // the legacy path, by fallback
			runs := map[string]*bbEnv{"default": e, "legacy": moved, "env": e.with("EXCHANGE_ONLINE_CONFIG=" + named)}
			read := []map[string]any{{"name": "exchange-online_read", "arguments": map[string]any{"endpoint_id": "mailboxes.list", "params": map[string]any{"tenantId": bbTenant}}}}
			for how, env := range runs {
				outs := []string{env.run(t, "version")}
				if strings.HasPrefix(name, "url") {
					// The file's own base decides, and doctor prints it. Nothing listens
					// there, so the slower commands only retry.
					outs = append(outs, env.without("EXCHANGE_ONLINE_BASE_URL").run(t, "doctor", "--json"))
				} else {
					outs = append(outs, env.run(t, "mailboxes", "list", bbTenant, "--json"), env.run(t, "doctor", "--json"), env.mcpCalls(t, read))
				}
				for _, out := range outs {
					noPiece(t, how+" startup", out, secret)
					if m := crash.FindString(out); m != "" {
						t.Errorf("%s: the program crashed on the file (%s):\n%.1200s", how, m, out)
					}
				}
			}
			if strings.HasPrefix(name, "url") {
				return
			}
			out := e.run(t, "mailboxes", "list", bbTenant, "--json", "--config", named)
			noPiece(t, "--config startup", out, secret)
		})
	}
}

// Class error-printed-before-the-filter: `auth set-token <key>` as an argument, in any
// spelling, and other error paths never show the key, and print once.
func TestErrorsPrintOnceAndNeverShowTheKey(t *testing.T) {
	const key = "PastedKeyOnTheLine8Zr3Yw"
	e, _ := listingAPI(t)
	for _, args := range [][]string{
		{"auth", "set-token", key},
		{"auth", "set-token", "--" + key},
		{"auth", "set-token", "--x=" + key},
		{"auth", "set-token", "--" + key + "=="},
		{"auth", "set-token", "-" + key},
		{"auth", "set-token", key, "extra"},
	} {
		out := e.runIn(t, "", args...)
		noPiece(t, "auth set-token <key>", out, key)
		if n := strings.Count(out, "Error:"); n > 1 {
			t.Errorf("%q printed its error %d times:\n%.800s", args[len(args)-1], n, out)
		}
	}
	noPiece(t, "a piped key with a pasted flag", e.runIn(t, key+"\n", "auth", "set-token", "--"+key), key)
	for _, args := range [][]string{
		{"mailboxes", "list", bbTenant, "--" + bbToken},
		{"mailboxes", "list", bbTenant, "--limit=" + bbToken},
		{bbToken},
		{"mailboxes", bbToken, "--json"},
	} {
		out := e.run(t, args...)
		noPiece(t, "an error path", out, bbToken)
		if n := strings.Count(out, "Error:"); n > 1 {
			t.Errorf("%q printed its error %d times:\n%.800s", args, n, out)
		}
	}
	out := e.run(t, "mailboxes", "list", bbTenant, "--no-such-flag")
	if n := strings.Count(out, "unknown flag: --no-such-flag"); n != 1 {
		t.Errorf("an ordinary error printed %d times, want once:\n%.800s", n, out)
	}
}

// Class help-waits-on-an-open-pipe: help reads no stdin, so a harness that leaves its pipe
// open is answered at once. A --help that is another flag's value is not help, and the
// piped value is still held.
func TestHelpDoesNotWaitForAnOpenPipe(t *testing.T) {
	e, _ := listingAPI(t)
	for _, args := range [][]string{
		{"auth", "set-token", "--help"},
		{"auth", "set-token", "-h"},
		{"auth", "set-token", "--help=true"},
		{"auth", "set-token", "--json", "--help"},
		{"auth", "--help", "set-token"},
		{"help", "auth", "set-token"},
		{"mailboxes", "set", bbTenant, "--stdin", "--help"},
	} {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			t.Parallel()
			cmd := exec.Command(bbCLI, args...)
			cmd.Env, cmd.Dir = e.env, e.home
			stdin, err := cmd.StdinPipe() // open for the whole run, with nothing written
			if err != nil {
				t.Fatal(err)
			}
			defer stdin.Close()
			var out bytes.Buffer
			cmd.Stdout, cmd.Stderr = &out, &out
			if err := cmd.Start(); err != nil {
				t.Fatal(err)
			}
			done := make(chan error, 1)
			go func() { done <- cmd.Wait() }()
			select {
			case err := <-done:
				if err != nil || !strings.Contains(out.String(), "Usage:") {
					t.Errorf("exit %v, want the help:\n%.800s", err, out.String())
				}
			case <-time.After(20 * time.Second):
				_ = cmd.Process.Kill()
				<-done
				t.Errorf("still waiting on the open pipe after 20 s")
			}
		})
	}
	const piped = "PipedKeyAfterAHelpWord3Tq8"
	for _, args := range [][]string{
		{"auth", "set-token", "--help"},
		{"auth", "set-token", "--help=false"},
		{"auth", "set-token", "--config", "--help"},
		{"auth", "set-token", "--", "--help"},
	} {
		noPiece(t, strings.Join(args, " "), e.runIn(t, piped+"\n", args...), piped)
	}
}

// Class mcp-stderr-lost-at-exit: a refused HTTP start prints its reason, through the
// built binary, before the process exits.
func TestMCPRefusedHTTPStartPrintsItsReason(t *testing.T) {
	e, _ := listingAPI(t)
	for _, tc := range []struct {
		name string
		env  *bbEnv
		args []string
		want string
	}{
		{"no token", e.without("EXCHANGE_ONLINE_MCP_HTTP_TOKEN"), nil, "must be set to a bearer token"},
		{"short token", e.without("EXCHANGE_ONLINE_MCP_HTTP_TOKEN").with("EXCHANGE_ONLINE_MCP_HTTP_TOKEN=short"), nil, "must be set to a bearer token"},
		{"non-loopback without TLS", e, []string{"--addr", "203.0.113.9:18080"}, "requires --tls-cert and --tls-key"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			for i := 0; i < 5; i++ { // the race this closes loses only sometimes
				cmd := exec.Command(bbMCP, append([]string{"--transport", "http"}, tc.args...)...)
				cmd.Env, cmd.Dir = tc.env.env, tc.env.home
				b, err := cmd.CombinedOutput()
				if err == nil {
					t.Fatalf("a refused start exited 0:\n%s", b)
				}
				if !strings.Contains(string(b), tc.want) {
					t.Fatalf("run %d: the reason was lost (%v): %q", i, err, b)
				}
			}
		})
	}
}

// Class doctor-truth-and-verdict-word: doctor --json shows its credentials verdict word
// in clear, tells a rejected token from a good one, and a real credential is withheld.
func TestDoctorShowsVerdictInClear(t *testing.T) {
	status := atomic.Int32{}
	status.Store(http.StatusOK)
	srv := tlsServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method == http.MethodPost {
			w.WriteHeader(int(status.Load()))
		}
		_, _ = w.Write([]byte(`{"value":[{"DomainName":"contoso.example"}]}`))
	}))
	ca := pemFile(t, srv)
	e := newEnv(t, &server{url: srv.URL, caFile: ca}, true)
	row := func() string {
		t.Helper()
		cmd := exec.Command(bbCLI, "doctor", "--json")
		cmd.Env, cmd.Dir = e.env, e.home
		stdout, _ := cmd.Output()
		var report map[string]any
		if err := json.Unmarshal(stdout, &report); err != nil {
			t.Fatalf("doctor --json is not JSON: %v\n%s", err, stdout)
		}
		noPiece(t, "doctor --json", string(stdout), bbToken)
		got, _ := report["credentials"].(string)
		if got == "" || strings.Contains(got, "[redacted]") || strings.Contains(string(stdout), bbWithheld) {
			t.Fatalf("the verdict is not in clear: %q\n%s", got, stdout)
		}
		return got
	}
	if got := row(); !strings.HasPrefix(got, "valid") {
		t.Errorf("a good token reads %q, want valid", got)
	}
	status.Store(http.StatusUnauthorized)
	if got := row(); !strings.Contains(got, "rejected") {
		t.Errorf("a rejected token reads %q", got)
	}
	// A real credential on the same report is still withheld.
	out := e.with("EXCHANGE_ONLINE_BASE_URL="+srv.URL+"/?token="+bbToken).run(t, "doctor", "--json")
	noPiece(t, "doctor with a token in the base URL", out, bbToken)
}

// Class read-properties-after-the-fragment: exchange-online_read takes properties, which
// the Exchange admin API reads as $select on the query string, with the cmdlet still in
// the envelope.
func TestReadAcceptsProperties(t *testing.T) {
	var mu sync.Mutex
	var selects, cmdlets, fragments []string
	srv := tlsServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		mu.Lock()
		selects = append(selects, r.URL.Query().Get("$select"))
		cmdlets = append(cmdlets, cmdletOf(body))
		fragments = append(fragments, r.URL.RawQuery)
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"value":[{"Identity":"rosa@example.com","DisplayName":"Rosa Diaz"}]}`))
	}))
	e := newEnv(t, &server{url: srv.URL, caFile: pemFile(t, srv)}, true)
	out := e.mcpCalls(t, []map[string]any{{"name": "exchange-online_read", "arguments": map[string]any{
		"endpoint_id": "mailboxes.list",
		"params":      map[string]any{"tenantId": bbTenant, "properties": "Identity,DisplayName"},
	}}})
	if strings.Contains(out, "invalid cmdlet name") || !strings.Contains(out, "Rosa Diaz") {
		t.Fatalf("the read refused properties:\n%.1200s", out)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(selects) != 1 || selects[0] != "Identity,DisplayName" || cmdlets[0] != "Get-Mailbox" {
		t.Errorf("want one Get-Mailbox with $select=Identity,DisplayName, got select %q cmdlet %q query %q", selects, cmdlets, fragments)
	}
}

// Class example-loses-its-columns: --agent compaction drops the columns --properties asked
// for unless --select names them too. The help's own Example, run as printed, shows every
// column it asks for.
func TestMailboxesListExampleShowsItsColumns(t *testing.T) {
	srv := tlsServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"value":[{"UserPrincipalName":"rosa@example.com","RecipientTypeDetails":"UserMailbox","ForwardingSmtpAddress":"smtp:rosa@vendor.example"}]}`))
	}))
	e := newEnv(t, &server{url: srv.URL, caFile: pemFile(t, srv)}, true)
	help := e.run(t, "mailboxes", "list", "--help")
	line := regexp.MustCompile(`exchange-online-cli mailboxes list \S+ --properties (\S+) --agent.*`).FindStringSubmatch(help)
	if line == nil {
		t.Fatalf("no --properties --agent Example in the help:\n%.1200s", help)
	}
	out := e.run(t, strings.Fields(line[0])[1:]...)
	for _, column := range strings.Split(line[1], ",") {
		if !strings.Contains(out, column) {
			t.Errorf("the Example %q lost the column %s:\n%.800s", line[0], column, out)
		}
	}
}

// Class mcp-preview-passes-file-flags: a write without confirm previews, first refusing
// any host-file flag with an error naming it, and passes ordinary flags through;
// execute previews every non-Get cmdlet until confirm is boolean true; offboard
// previews with its plan; read still works under EXCHANGE_ONLINE_MCP_READ_ONLY=1.
func TestMCPConfirmPreviews(t *testing.T) {
	e, writes := listingAPI(t)
	file := filepath.Join(e.home, "host-file.txt")
	target := map[string]any{"tenantId": bbTenant, "mailbox": "rosa@example.com"}
	with := func(k string, v any) map[string]any {
		m := map[string]any{k: v}
		for a, b := range target {
			m[a] = b
		}
		return m
	}
	for _, flag := range []string{"config", "home", "deliver", "receipt-file", "audit-dir", "ca-cert", "profile"} {
		out := e.mcpCalls(t, []map[string]any{{"name": "offboard", "arguments": with(flag, file)}})
		if !strings.Contains(out, `"isError":true`) || !strings.Contains(out, flag) {
			t.Errorf("an unconfirmed offboard took --%s:\n%.1000s", flag, out)
		}
		if strings.Contains(out, "Preview only") {
			t.Errorf("--%s reached a preview:\n%.1000s", flag, out)
		}
	}
	if _, err := os.Stat(file); err == nil {
		t.Errorf("a preview wrote the host file")
	}
	out := e.mcpCalls(t, []map[string]any{{"name": "offboard", "arguments": with("select", "mode,steps")}})
	if !strings.Contains(out, "Preview only") || strings.Contains(out, `"isError":true`) {
		t.Errorf("an ordinary flag did not pass through the preview:\n%.1200s", out)
	}
	out = e.mcpCalls(t, []map[string]any{{"name": "offboard", "arguments": target}})
	if !strings.Contains(out, `plan`) {
		t.Errorf("offboard did not preview with its plan:\n%.1200s", out)
	}
	set := map[string]any{"tenantId": bbTenant, "Identity": "rosa@example.com", "Type": "Shared"}
	for _, confirm := range []any{nil, false, "true", 1} {
		args := map[string]any{"endpoint_id": "mailboxes.set", "params": set}
		if confirm != nil {
			args["confirm"] = confirm
		}
		out := e.mcpCalls(t, []map[string]any{{"name": "exchange-online_execute", "arguments": args}})
		if !strings.Contains(out, "Preview only") && !strings.Contains(out, "preview") {
			t.Errorf("execute with confirm=%v did not preview:\n%.1000s", confirm, out)
		}
	}
	if n := writes.Load(); n != 0 {
		t.Errorf("%d write(s) reached the API without confirm", n)
	}
	e.mcpCalls(t, []map[string]any{{"name": "exchange-online_execute", "arguments": map[string]any{"endpoint_id": "mailboxes.set", "params": set, "confirm": true}}})
	if n := writes.Load(); n != 1 {
		t.Errorf("confirm: true sent %d write(s), want 1", n)
	}
	ro := e.with("EXCHANGE_ONLINE_MCP_READ_ONLY=1").mcpCalls(t, []map[string]any{{"name": "exchange-online_read", "arguments": map[string]any{"endpoint_id": "mailboxes.list", "params": map[string]any{"tenantId": bbTenant}}}})
	if !strings.Contains(ro, "Rosa Diaz") {
		t.Errorf("exchange-online_read refused under EXCHANGE_ONLINE_MCP_READ_ONLY=1:\n%.1000s", ro)
	}
}

// Class journal-credential: the learn journal holds no credential. The session label
// is stored hashed, and a token pasted as a flag name leaves only a placeholder, while
// declared flag names still land.
func TestJournalHoldsNoCredential(t *testing.T) {
	const label = "HarnessSessionLabel7Qk2Xw"
	e, _ := listingAPI(t)
	s := e.with("EXCHANGE_ONLINE_LEARN_SESSION=" + label)
	s.run(t, "mailboxes", "list", bbTenant, "--json")
	s.run(t, "mailboxes", "list", bbTenant, "--"+bbToken)
	s.run(t, "mailboxes", "list", bbTenant, "--"+bbToken+"=x", "--json")
	s.run(t, "mailboxes", "list", bbTenant, "--limit", bbToken)
	journal := ""
	_ = filepath.WalkDir(e.home, func(p string, d os.DirEntry, err error) error {
		if err == nil && !d.IsDir() && filepath.Base(filepath.Dir(p)) == "journal" || err == nil && !d.IsDir() && strings.HasPrefix(d.Name(), "journal") {
			b, _ := os.ReadFile(p)
			journal += string(b)
		}
		return nil
	})
	if !strings.Contains(journal, `"argv_shape"`) {
		t.Fatalf("no journal was written under %s, so this proves nothing:\n%.500s", e.home, journal)
	}
	noPiece(t, "the journal (token)", journal, bbToken)
	noPiece(t, "the journal (session label)", journal, label)
	if !strings.Contains(journal, `"json"`) {
		t.Errorf("a declared flag was lost:\n%.800s", journal)
	}
}
