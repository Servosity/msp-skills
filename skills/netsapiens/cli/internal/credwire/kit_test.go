// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

// Hand-written (NOT generated): the credential classes the kit closes for this
// connector, each driven through the built binaries from a review's own input.
// Every test searches all the run printed, answered or left on disk.

package credwire

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"netsapiens-pp-cli/internal/cli"
	"netsapiens-pp-cli/internal/cliutil"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

var built struct {
	once     sync.Once
	cli, mcp string
	err      error
}

func binaries(t *testing.T) (string, string) {
	t.Helper()
	if testing.Short() {
		t.Skip("builds and runs both binaries")
	}
	if runtime.GOOS == "windows" {
		t.Skip("drives the binaries through POSIX pipes; the kit's Windows stdio is proven by the kit's own tests")
	}
	built.once.Do(func() {
		dir, err := os.MkdirTemp("", "netsapiens-credwire-")
		if err != nil {
			built.err = err
			return
		}
		built.cli, built.mcp = filepath.Join(dir, "netsapiens-cli"), filepath.Join(dir, "netsapiens-mcp")
		for out, pkg := range map[string]string{built.cli: "../../cmd/netsapiens-cli", built.mcp: "../../cmd/netsapiens-mcp"} {
			if b, err := exec.Command("go", "build", "-o", out, pkg).CombinedOutput(); err != nil {
				built.err = &buildError{err, string(b)}
				return
			}
		}
	})
	if built.err != nil {
		t.Fatal(built.err)
	}
	return built.cli, built.mcp
}

type buildError struct {
	err error
	out string
}

func (e *buildError) Error() string { return e.err.Error() + "\n" + e.out }

const fixtureCredential = "KitFixtureValue99887766"

// bbEnv is one throwaway home and the variables that point a binary at it.
type bbEnv struct {
	home string
	env  []string
}

func newEnv(t *testing.T, extra ...string) *bbEnv {
	t.Helper()
	binaries(t)
	home := t.TempDir()
	env := []string{"HOME=" + home, "USERPROFILE=" + home, "NETSAPIENS_HOME=" + filepath.Join(home, "ns"), "NETSAPIENS_RETRY_MAX=0"}
	for _, kv := range os.Environ() {
		name, _, _ := strings.Cut(kv, "=")
		if name == "PATH" || name == "TMPDIR" {
			env = append(env, kv)
		}
	}
	return &bbEnv{home: home, env: append(env, extra...)}
}

// vendor is a fake NetSapiens server over https and the variables that trust it.
func (e *bbEnv) vendor(t *testing.T, h http.HandlerFunc) *httptest.Server {
	t.Helper()
	srv := httptest.NewTLSServer(h)
	t.Cleanup(srv.Close)
	ca := filepath.Join(t.TempDir(), "ca.pem")
	if err := os.WriteFile(ca, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: srv.Certificate().Raw}), 0o600); err != nil {
		t.Fatal(err)
	}
	e.env = append(e.env, "NETSAPIENS_CA_CERT="+ca, "SSL_CERT_FILE="+ca)
	return srv
}

func (e *bbEnv) run(t *testing.T, stdin string, args ...string) string {
	t.Helper()
	cliBin, _ := binaries(t)
	cmd := exec.Command(cliBin, args...)
	cmd.Env = e.env
	cmd.Stdin = strings.NewReader(stdin)
	var out bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &out
	_ = cmd.Run()
	return out.String()
}

func (e *bbEnv) files(t *testing.T) string {
	t.Helper()
	var all strings.Builder
	_ = filepath.Walk(e.home, func(p string, info os.FileInfo, err error) error {
		if err == nil && info.Mode().IsRegular() && !strings.HasSuffix(p, "credentials.toml") && !strings.HasSuffix(p, "config.toml") {
			b, _ := os.ReadFile(p)
			all.Write(b)
		}
		return nil
	})
	return all.String()
}

func writeFile(t *testing.T, path, body string) string {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func jsonList(body string) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, body)
	}
}

// credential-holder (publish round 2): a valid inline headers table is a held
// credential, so a playbook note repeating it never prints. The local store
// keeps the note (synccpu, owner GO 2026-11-09: the store is not checked); the
// kit acting on the list that would print it proves the value is held.
func TestKitHoldsInlineAndNestedConfigTables(t *testing.T) {
	const secret = "InlineHeaderKey55443322"
	for name, cfg := range map[string]string{
		"inline table": `headers = { "X-Api-Key" = "` + secret + `", Authorization = "Bearer ` + secret + `" }` + "\n",
		"nested table": "[headers]\n\"X-Api-Key\" = \"" + secret + "\"\n",
		"array":        `extra = ["` + secret + `"]` + "\n",
		"unterminated": "bad = [\napi_key = \"" + secret + "\"\n",
	} {
		t.Run(name, func(t *testing.T) {
			e := newEnv(t)
			c := writeFile(t, filepath.Join(e.home, "ns.toml"), cfg)
			notes := writeFile(t, filepath.Join(e.home, "notes", "n.md"), "rotate "+secret+"\n")
			e.env = append(e.env, "NETSAPIENS_CONFIG="+c)
			out := e.run(t, "", "teach-playbook", "--query", "find offline devices", "--notes-file", notes)
			if strings.Contains(out, secret) {
				t.Fatalf("printed:\n%s", out)
			}
			// The kit acts on the value only because it holds it: the list
			// carrying the note is withheld whole, or startup refuses.
			out = e.run(t, "", "playbook", "list", "--json")
			if strings.Contains(out, secret) || !strings.Contains(out, "[credkit: ") {
				t.Fatalf("want the kit to withhold the playbook list carrying the note, got:\n%s", out)
			}
		})
	}
}

// A vendor credential field is redacted and the list still prints; repeated in
// another field the unit is withheld whole. The sibling row's data survives only
// in the first case, which proves the redaction is not a blanket refusal.
func TestKitDeclaredResponseFields(t *testing.T) {
	const smtp = "SmtpSecret55443322"
	cases := []struct {
		name, body string
		wantShown  string
	}{
		{"named field is redacted, list prints", `[{"domain":"acme.example","email-send-smtp-password":"` + smtp + `"}]`, "acme.example"},
		{"repeated elsewhere is withheld", `[{"domain":"acme.example","email-send-smtp-password":"` + smtp + `","description":"pw ` + smtp + `"}]`, ""},
		{"credential in text", `[{"domain":"acme.example","description":"device-sip-registration-password=` + smtp + `"}]`, "acme.example"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			e := newEnv(t, "NETSAPIENS_API_KEY="+fixtureCredential)
			srv := e.vendor(t, jsonList(tc.body))
			e.env = append(e.env, "NETSAPIENS_BASE_URL="+srv.URL)
			out := e.run(t, "", "domains", "list", "--json", "--no-cache", "--data-source", "live")
			if strings.Contains(out, smtp) {
				t.Fatalf("printed the vendor secret:\n%s", out)
			}
			if tc.wantShown != "" && !strings.Contains(out, tc.wantShown) {
				t.Fatalf("want %q in:\n%s", tc.wantShown, out)
			}
			if strings.Contains(e.files(t), smtp) {
				t.Fatal("a file holds the vendor secret")
			}
		})
	}
}

// meeting-registration-id is a webinar guest's token and refresh a refresh JWT
// (spec.json), under names the word rule cannot read. A JWT is one under any name.
func TestKitDocumentedAndShapedCredentials(t *testing.T) {
	const guest = "WebinarJoinKey55443322"
	const refresh = "RefreshGrant55443322"
	enc := base64.RawURLEncoding.EncodeToString
	jwt := enc([]byte(`{"alg":"RS256","typ":"JWT"}`)) + "." + enc([]byte(`{"sub":"100@acme.example","exp":4102444800}`)) + "." + enc(bytes.Repeat([]byte("signature!"), 8))
	e := newEnv(t, "NETSAPIENS_API_KEY="+fixtureCredential)
	srv := e.vendor(t, jsonList(`[{"id":"m1","meeting-registration-id":"`+guest+`","refresh":"`+refresh+`","scope":"`+jwt+`"}]`))
	e.env = append(e.env, "NETSAPIENS_BASE_URL="+srv.URL)
	out := e.run(t, "", "domains", "users", "meetings-list", "--domain", "acme.example", "--user", "100", "--json")
	for _, s := range []string{guest, refresh, jwt} {
		if strings.Contains(out, s) || strings.Contains(e.files(t), s) {
			t.Fatalf("%q reached an output:\n%s", s, out)
		}
	}
	if !strings.Contains(out, "m1") {
		t.Fatalf("the row itself was lost:\n%s", out)
	}
}

// A short numeric PIN is an Identifier on the response side: redacted where named,
// and other numbers in the same list stay readable.
func TestKitPinIsRedactedAndNotHeld(t *testing.T) {
	e := newEnv(t, "NETSAPIENS_API_KEY="+fixtureCredential)
	srv := e.vendor(t, jsonList(`[{"domain":"acme.example","user":"1234","voicemail-login-pin":"1234"}]`))
	e.env = append(e.env, "NETSAPIENS_BASE_URL="+srv.URL)
	out := e.run(t, "", "domains", "list", "--json", "--no-cache", "--data-source", "live")
	if !strings.Contains(out, `"user"`) || !strings.Contains(out, "1234") {
		t.Fatalf("the extension was withheld with the PIN:\n%s", out)
	}
	var doc struct {
		Results []map[string]string `json:"results"`
	}
	if err := json.Unmarshal([]byte(out), &doc); err != nil || len(doc.Results) != 1 {
		t.Fatalf("no result (%v):\n%s", err, out)
	}
	if got := doc.Results[0]["voicemail-login-pin"]; got != cliutil.RedactedValue {
		t.Fatalf("the PIN printed as %q", got)
	}
}

// credential-routing: the key goes only to the configured server, never over plain
// http and never across a redirect to another origin.
func TestKitCredentialStaysOnTheConfiguredOrigin(t *testing.T) {
	const routed = "RoutingKey99887766"
	t.Run("plain http is refused", func(t *testing.T) {
		var hits atomic.Int32
		srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { hits.Add(1) }))
		defer srv.Close()
		e := newEnv(t, "NETSAPIENS_API_KEY="+routed, "NETSAPIENS_BASE_URL="+srv.URL)
		e.run(t, "", "domains", "list", "--json", "--no-cache", "--data-source", "live")
		if hits.Load() != 0 {
			t.Fatalf("%d request(s) over plain http", hits.Load())
		}
	})
	t.Run("a redirect to another origin is refused", func(t *testing.T) {
		var otherHits atomic.Int32
		e := newEnv(t, "NETSAPIENS_API_KEY="+routed)
		other := e.vendor(t, func(http.ResponseWriter, *http.Request) { otherHits.Add(1) })
		srv := httptest.NewUnstartedServer(http.RedirectHandler(other.URL+"/steal", http.StatusFound))
		srv.TLS = other.TLS.Clone()
		srv.StartTLS()
		defer srv.Close()
		e.env = append(e.env, "NETSAPIENS_BASE_URL="+srv.URL)
		e.run(t, "", "domains", "list", "--json", "--no-cache", "--data-source", "live")
		if otherHits.Load() != 0 {
			t.Fatalf("%d request(s) followed the redirect", otherHits.Load())
		}
	})
	t.Run("an explicit --config picks the server, not a discovered default", func(t *testing.T) {
		var defaultHits, chosenHits atomic.Int32
		e := newEnv(t, "NETSAPIENS_API_KEY="+routed)
		chosen := e.vendor(t, func(w http.ResponseWriter, _ *http.Request) {
			chosenHits.Add(1)
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, `[]`)
		})
		def := httptest.NewUnstartedServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { defaultHits.Add(1) }))
		def.TLS = chosen.TLS.Clone()
		def.StartTLS()
		defer def.Close()
		writeFile(t, filepath.Join(e.home, "ns", "config", "config.toml"), `base_url = "`+def.URL+`"`+"\n")
		c := writeFile(t, filepath.Join(e.home, "chosen.toml"), `base_url = "`+chosen.URL+`"`+"\n")
		out := e.run(t, "", "domains", "list", "--config", c, "--json", "--no-cache", "--data-source", "live")
		if defaultHits.Load() != 0 {
			t.Fatalf("the discovered default got %d request(s)", defaultHits.Load())
		}
		if chosenHits.Load() == 0 {
			t.Fatalf("the chosen server got nothing:\n%s", out)
		}
	})
}

// A --stdin body's credentials are held before anything prints, a preview included.
func TestKitStdinBodyIsHeldBeforeThePreview(t *testing.T) {
	const sip = "StdinSipSecret55443322"
	e := newEnv(t, "NETSAPIENS_API_KEY="+fixtureCredential, "NETSAPIENS_SERVER=ns.example.com")
	body := `{"device":"100","device-sip-registration-password":"` + sip + `"}`
	out := e.run(t, body, "domains", "users", "devices-create", "--domain", "acme.example", "--user", "100", "--device", "100", "--stdin", "--dry-run")
	if strings.Contains(out, sip) {
		t.Fatalf("the preview printed the body's credential:\n%s", out)
	}
	if !strings.Contains(out, "withheld") && !strings.Contains(out, "<redacted>") && !strings.Contains(out, "\\u003credacted") {
		t.Fatalf("no sign the command reached the body:\n%s", out)
	}
	if strings.Contains(e.files(t), sip) {
		t.Fatal("a file holds the body's credential")
	}
}

// A request body passed to --body-json is held before the preview prints it: by
// the kit's HoldBody at startup, and again by the client's hold of what it sends.
func TestKitBodyJSONIsHeldBeforeThePreview(t *testing.T) {
	const pass = "BodyJsonPassword55443322"
	e := newEnv(t, "NETSAPIENS_API_KEY="+fixtureCredential, "NETSAPIENS_SERVER=ns.example.com")
	out := e.run(t, "", "domains", "conferences", "create", "--domain", "acme.example",
		"--body-json", `[{"name":"Main","password":"`+pass+`"}]`, "--dry-run")
	if strings.Contains(out, pass) {
		t.Fatalf("the preview printed the body's credential:\n%s", out)
	}
	if !strings.Contains(out, "withheld") && !strings.Contains(out, "<redacted>") && !strings.Contains(out, "\\u003credacted") {
		t.Fatalf("no sign the command reached the body:\n%s", out)
	}
}

// A credential flag is held before a flag error can repeat it.
func TestKitArgvCredentials(t *testing.T) {
	e := newEnv(t, "NETSAPIENS_SERVER=ns.example.com")
	const pass = "ArgvPassword55443322"
	out := e.run(t, "", "domains", "users", "create", "--password="+pass, "--nonsense-flag")
	if strings.Contains(out, pass) {
		t.Fatalf("printed:\n%s", out)
	}
}

// profiles.json is a source: a saved value under a credential flag's name is held
// at startup through the kit, every duplicate included (the parser keeps only the
// last), so a description repeating it never prints. Each profile mentions one
// value, and no two values share a piece of 8, so each is proved on its own. An
// ordinary profile still prints whole.
func TestKitProfileStore(t *testing.T) {
	const first, last, access = "DupFirstPw10293847", "DupLastPw56473829", "AccessTk91827364"
	e := newEnv(t, "NETSAPIENS_SERVER=ns.example.com")
	store := `{"profiles":{` +
		`"dup":{"name":"dup","description":"rotate ` + first + `","values":{"password":"` + first + `","password":"` + last + `"}},` +
		`"last":{"name":"last","description":"rotate ` + last + `","values":{"limit":"7"}},` +
		`"access":{"name":"access","description":"rotate ` + access + `","values":{"access-token":"` + access + `"}},` +
		`"plain":{"name":"plain","description":"nightly offline report","values":{"limit":"7"}}}}`
	writeFile(t, filepath.Join(e.home, "ns", "config", "profiles.json"), store)
	for _, name := range []string{"dup", "last", "access"} {
		for _, args := range [][]string{{"profile", "show", name, "--json"}, {"profile", "use", name}} {
			out := e.run(t, "", args...)
			for _, s := range []string{first, last, access} {
				if strings.Contains(out, s) {
					t.Fatalf("%v printed %q:\n%s", args, s, out)
				}
			}
		}
	}
	if out := e.run(t, "", "profile", "use", "plain"); !strings.Contains(out, "nightly offline report") || !strings.Contains(out, "--limit 7") {
		t.Fatalf("an ordinary profile was withheld:\n%s", out)
	}
}

// mcpCall runs one tools/call over stdio against the built server.
func mcpCall(t *testing.T, env []string, name string, args map[string]any) string {
	t.Helper()
	_, mcpBin := binaries(t)
	cmd := exec.Command(mcpBin)
	cmd.Env = env
	stdin, _ := cmd.StdinPipe()
	var out bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &out
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	call, _ := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": 2, "method": "tools/call", "params": map[string]any{"name": name, "arguments": args}})
	_, _ = io.WriteString(stdin, `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18","capabilities":{},"clientInfo":{"name":"t","version":"1"}}}`+"\n")
	_, _ = io.WriteString(stdin, `{"jsonrpc":"2.0","method":"notifications/initialized"}`+"\n"+string(call)+"\n")
	done := make(chan struct{})
	go func() { _ = cmd.Wait(); close(done) }()
	time.Sleep(3 * time.Second)
	_ = stdin.Close()
	select {
	case <-done:
	case <-time.After(30 * time.Second):
		_ = cmd.Process.Kill()
		<-done
	}
	return out.String()
}

// An MCP credential argument is held before dispatch, so a preview never repeats it.
func TestKitMCPCredentialArgument(t *testing.T) {
	const sip = "McpSipSecret55443322"
	e := newEnv(t, "NETSAPIENS_API_KEY="+fixtureCredential, "NETSAPIENS_SERVER=ns.example.com")
	got := mcpCall(t, e.env, "netsapiens_execute", map[string]any{
		"endpoint_id": "domains.users.devices-create",
		"params":      map[string]any{"device-sip-registration-password": sip},
	})
	if strings.Contains(got, sip) || !strings.Contains(got, `"id":2`) {
		t.Fatalf("the tool answer repeats the argument, or there was no answer:\n%s", got)
	}
}

// fleet_changes is advertised read-only: over MCP --rebaseline is refused, so an
// agent cannot erase what the next comparison would report.
func TestMCPCannotMoveTheBaseline(t *testing.T) {
	cliBin, _ := binaries(t)
	e := newEnv(t, "NETSAPIENS_API_KEY="+fixtureCredential, "NETSAPIENS_CLI_PATH="+cliBin)
	var body atomic.Value
	body.Store(`[{"domain":"a.example","description":"Old"}]`)
	srv := e.vendor(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if strings.HasSuffix(r.URL.Path, "/domains") {
			_, _ = io.WriteString(w, body.Load().(string))
			return
		}
		_, _ = io.WriteString(w, `[]`)
	})
	e.env = append(e.env, "NETSAPIENS_BASE_URL="+srv.URL)
	e.run(t, "", "sync", "--resources", "domains")
	e.run(t, "", "fleet", "changes", "--rebaseline", "--json")
	body.Store(`[{"domain":"a.example","description":"New"}]`)
	e.run(t, "", "sync", "--resources", "domains")
	got := mcpCall(t, e.env, "fleet_changes", map[string]any{"rebaseline": true})
	if !strings.Contains(got, "CLI only") {
		t.Fatalf("the tool did not refuse:\n%s", got)
	}
	out := e.run(t, "", "fleet", "changes", "--json")
	if !strings.Contains(out, "New") {
		t.Fatalf("the change is gone from the comparison, so the baseline moved:\n%s", out)
	}
}

// The declarations and the connector's own walker are one rule.
func TestDeclaredNamesMatchTheWalker(t *testing.T) {
	for _, name := range append(slices.Clone(Declarations.ResponseFields), Declarations.CredentialFields...) {
		if !cliutil.IsSecretField(name) {
			t.Errorf("field %s is declared but the walker would print it", name)
		}
	}
	for _, name := range []string{"domain", "user", "description", "language-token", "token-endpoint", "key-id"} {
		if cliutil.IsSecretField(name) {
			t.Errorf("%s is not a credential", name)
		}
	}
}

// Every PIN response field is an Identifier and no other credential field is.
func TestPinFieldsAreTheOnlyResponseExemptions(t *testing.T) {
	var exempt []string
	for _, id := range Declarations.Identifiers {
		if f, ok := strings.CutPrefix(id.Source, "response:"); ok {
			exempt = append(exempt, f)
			if !cliutil.IsPinField(f) {
				t.Errorf("response field %s is exempt but is no PIN", f)
			}
			if slices.Contains(Declarations.ResponseFields, f) {
				t.Errorf("response field %s is both declared and exempt", f)
			}
		}
	}
	for _, f := range Declarations.CredentialFields {
		if cliutil.IsPinField(f) && !slices.Contains(exempt, f) {
			t.Errorf("PIN field %s has no Identifier", f)
		}
	}
}

// Every credential flag's name is a name HoldBody reads, so a saved profile value
// under it is held.
func TestCredentialFlagNamesAreBodyNames(t *testing.T) {
	names := slices.Concat(Declarations.CredentialFields, Declarations.ConfigFields, Declarations.StoredRecords, Declarations.MCPArguments)
	for _, f := range Declarations.CredentialFlags {
		if !slices.ContainsFunc(names, func(n string) bool { return strings.EqualFold(n, strings.TrimLeft(f, "-")) }) {
			t.Errorf("credential flag %s names no field HoldBody reads", f)
		}
	}
}

// Every credential-named flag any command declares is a declared credential flag.
func TestDeclaredFlagsCoverTheCommandTree(t *testing.T) {
	var walk func(*cobra.Command)
	var missing []string
	seen := 0
	ref := regexp.MustCompile(`credential-ref$`) // the name of a stored credential, not a credential
	walk = func(c *cobra.Command) {
		visit := func(f *pflag.Flag) {
			if f.Value.Type() != "string" || !cliutil.IsSecretField(f.Name) || ref.MatchString(f.Name) {
				return
			}
			seen++
			if !slices.Contains(Declarations.CredentialFlags, "--"+f.Name) {
				missing = append(missing, c.CommandPath()+" --"+f.Name)
			}
		}
		c.Flags().VisitAll(visit)
		c.PersistentFlags().VisitAll(visit)
		for _, sub := range c.Commands() {
			walk(sub)
		}
	}
	walk(cli.RootCmd())
	if seen == 0 {
		t.Fatal("no credential flag found, so this test proved nothing")
	}
	if len(missing) > 0 {
		t.Errorf("credential flags not declared: %q", missing)
	}
}

func TestAfterStartRestoresTheBaseURL(t *testing.T) {
	prior := "https://prior.example"
	for name, set := range map[string]bool{"unset": false, "set": true} {
		t.Run(name, func(t *testing.T) {
			t.Setenv("NETSAPIENS_HOME", t.TempDir())
			t.Setenv("NETSAPIENS_SERVER", "ns.example.com")
			t.Setenv("NETSAPIENS_BASE_URL", "")
			os.Unsetenv("NETSAPIENS_BASE_URL")
			if set {
				os.Setenv("NETSAPIENS_BASE_URL", prior)
			}
			saved := cliutil.Hold
			cliutil.Hold = func(string, ...string) {}
			defer func() { cliutil.Hold = saved }()
			showBaseURL("", "")
			if got := os.Getenv("NETSAPIENS_BASE_URL"); set && got != prior || !set && got != "https://ns.example.com/ns-api/v2" {
				t.Fatalf("during Start: %q", got)
			}
			AfterStart()
			got, ok := os.LookupEnv("NETSAPIENS_BASE_URL")
			if !set && ok {
				t.Fatalf("left set to %q", got)
			}
			if set && got != prior {
				t.Fatalf("got %q, want %q", got, prior)
			}
		})
	}
}
