// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package keeperblackbox

// The classes the fleet reviews named on this connector, one test each, from its
// failing input, driven through the built binaries wired to the shared credential kit
// (handfixes.json credkit-wiring). .review/failing-first.txt records how each behaved
// on the tree before the kit.

import (
	"bytes"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	_ "modernc.org/sqlite"
)

// fullwidth spells ASCII letters and digits in their fullwidth forms, which NFKC reads
// back.
func fullwidth(s string) string {
	var b strings.Builder
	for _, r := range s {
		switch {
		case r >= '0' && r <= '9':
			b.WriteRune(0xFF10 + r - '0')
		case r >= 'A' && r <= 'Z':
			b.WriteRune(0xFF21 + r - 'A')
		case r >= 'a' && r <= 'z':
			b.WriteRune(0xFF41 + r - 'a')
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}

// percentEveryByte percent-encodes every byte of s.
func percentEveryByte(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		fmt.Fprintf(&b, "%%%02X", s[i])
	}
	return b.String()
}

// readCompanies reads one typed report on each binary: the CLI's stdout and stderr, and
// one MCP session.
func readCompanies(t *testing.T, e *bbEnv) (cli, mcp string) {
	t.Helper()
	return e.run(t, "companies", "list", "--json"),
		e.mcpCalls(t, []map[string]any{{"name": "companies_list", "arguments": map[string]any{}}})
}

// Class encoded-credential-leak. A read's answer carries an 8-character piece of the key
// percent-encoded over its fullwidth form, and separately as base64 of its hex. Neither
// spelling holds the key's text, so only the decoders of contract section 3 find it. The
// same spellings of a value nobody holds print as written, so a withheld answer is the
// piece, not the spelling.
func TestClass_encoded_credential_leak(t *testing.T) {
	t.Parallel()
	const notHeld = "Zp4Xc7Vb2Nm9"
	piece := bbToken[7:15]
	for name, encode := range map[string]func(string) string{
		"percent-encoded fullwidth": func(v string) string { return percentEveryByte(fullwidth(v)) },
		"base64 of hex":             func(v string) string { return base64.StdEncoding.EncodeToString([]byte(hex.EncodeToString([]byte(v)))) },
	} {
		t.Run(name, func(t *testing.T) {
			sv := echoServer(t)
			sv.honest.Store(true)
			e := newEnv(t, sv, true)
			clear := encode(notHeld)
			sv.show(clear)
			cli, mcp := readCompanies(t, e)
			for where, out := range map[string]string{"CLI": cli, "MCP": mcp} {
				if !strings.Contains(out, clear) || strings.Contains(out, bbWithheld) {
					t.Errorf("%s: the control %q was not printed as written:\n%.1200s", where, clear, out)
				}
			}
			leak := encode(piece)
			sv.show(leak)
			cli, mcp = readCompanies(t, e)
			for where, out := range map[string]string{"CLI": cli, "MCP": mcp} {
				if strings.Contains(out, leak) {
					t.Errorf("%s output carries the encoded piece %q:\n%.1200s", where, leak, out)
				}
				noPiece(t, where+" output", out, bbToken)
				if !strings.Contains(out, bbWithheld) {
					t.Errorf("%s output lacks the kit's notice:\n%.1200s", where, out)
				}
			}
		})
	}
}

// configDir is the CLI's config directory under e.home.
func configDir(t *testing.T, e *bbEnv) string {
	t.Helper()
	dir := filepath.Join(e.home, ".config", "keeper-cli")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	return dir
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

// Class partial-credential-leak. The key lives only under [headers] in config.toml, with
// nothing in the environment, and a report repeats 8 of its characters.
func TestClass_partial_credential_leak(t *testing.T) {
	t.Parallel()
	const headerKey = "Nb5Ky2H" + "w8Jd4Fs7Gc3Tr9Mv6Qx"
	piece := headerKey[6:14]
	check := func(t *testing.T, sv *server, where, control, withheld string) {
		t.Helper()
		if !strings.Contains(control, "Rq2Wd9Lk6Bz3") || strings.Contains(control, bbWithheld) {
			t.Errorf("%s: the control value was not printed:\n%.1200s", where, control)
		}
		if strings.Contains(withheld, piece) {
			t.Errorf("%s output carries the 8-character piece %q:\n%.1200s", where, piece, withheld)
		}
		if !strings.Contains(withheld, bbWithheld) {
			t.Errorf("%s output lacks the kit's notice:\n%.1200s", where, withheld)
		}
	}
	t.Run("headers only", func(t *testing.T) {
		sv := echoServer(t)
		sv.honest.Store(true)
		e := newEnv(t, sv, false)
		writeFile(t, filepath.Join(configDir(t, e), "config.toml"), "[headers]\napi-key = \""+headerKey+"\"\n")
		read := func() (string, string) {
			return e.run(t, "queue", "--json"), e.mcpCalls(t, []map[string]any{{"name": "queue_get-status", "arguments": map[string]any{}}})
		}
		sv.show("Rq2Wd9Lk6Bz3")
		ctlCLI, ctlMCP := read()
		if !sv.sawKey(headerKey) {
			t.Fatalf("the [headers] key never reached the server; the test did not run:\n%.800s", ctlCLI)
		}
		sv.show(piece)
		cli, mcp := read()
		check(t, sv, "CLI", ctlCLI, cli)
		check(t, sv, "MCP", ctlMCP, mcp)
	})
	t.Run("typed report beside an api_key", func(t *testing.T) {
		sv := echoServer(t)
		sv.honest.Store(true)
		e := newEnv(t, sv, false)
		writeFile(t, filepath.Join(configDir(t, e), "config.toml"), "api_key = \"Ls8Pn3Vc6Xt1Qz7Hb4Kj\"\n[headers]\napi-key = \""+headerKey+"\"\n")
		sv.show("Rq2Wd9Lk6Bz3")
		ctlCLI, ctlMCP := readCompanies(t, e)
		sv.show(piece)
		cli, mcp := readCompanies(t, e)
		check(t, sv, "CLI", ctlCLI, cli)
		check(t, sv, "MCP", ctlMCP, mcp)
	})
}

// Class outbound-split-credential-leak. A piece of a held key split across two fields of
// a request to another host, each field carrying 8 or more of its characters, is refused
// before it is sent: the sink is never contacted. Two details decide the shapes:
//
//   - A value under 8 characters in each field is out of contract (section 3 joins JSON
//     alone, section 6 item 6), so each half here is itself a piece the contract names.
//   - KEEPER_FEEDBACK_ENDPOINT is a URLEnv: every query value in it is held at startup,
//     whatever it is, so that sender refuses any query and cannot show the key is why. The
//     key's refusal on that sender is shown on its path; the query shape is still driven,
//     and --deliver, which no URLEnv reads, shows it for query fields.
func TestClass_outbound_split_credential_leak(t *testing.T) {
	t.Parallel()
	first, second := bbToken[:8], bbToken[8:16]
	var mu sync.Mutex
	var seen []string
	sink := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		auth, _ := base64.StdEncoding.DecodeString(strings.TrimPrefix(r.Header.Get("Authorization"), "Basic "))
		seen = append(seen, r.Method+" "+r.URL.RequestURI()+" ua="+r.UserAgent()+" auth="+string(auth))
		mu.Unlock()
		w.WriteHeader(http.StatusNoContent)
	}))
	t.Cleanup(sink.Close)
	take := func() string {
		mu.Lock()
		defer mu.Unlock()
		got := strings.Join(seen, "\n")
		seen = nil
		return got
	}
	sv := echoServer(t)
	sv.honest.Store(true)
	e := newEnv(t, sv, true)
	feedback := func(endpoint string) string {
		t.Helper()
		return e.with("KEEPER_FEEDBACK_ENDPOINT="+endpoint).run(t, "feedback", "a note", "--send")
	}
	deliver := func(url string, more ...string) string {
		t.Helper()
		return e.with(more...).run(t, "companies", "list", "--json", "--deliver", "webhook:"+url)
	}
	query := "/collect?a=" + first + "&b=" + second
	path := "/collect/" + first + "/" + second
	for where, out := range map[string]string{
		"feedback query fields": feedback(sink.URL + query),
		"feedback path":         feedback(sink.URL + path),
		"deliver query fields":  deliver(sink.URL + query),
		// Authorization (the URL's user) and User-Agent: two header fields.
		"deliver header fields": deliver(strings.Replace(sink.URL, "http://", "http://"+second+"@", 1)+"/collect", "KEEPER_USER_AGENT="+first),
		// A URL's user is refused over plain http whatever it is, so this is the half that
		// is about the key: the User-Agent alone, whose clean spelling arrives below.
		"deliver one header's half": deliver(sink.URL+"/collect", "KEEPER_USER_AGENT="+first),
	} {
		if strings.Contains(out, first) || strings.Contains(out, second) {
			t.Errorf("%s: the output repeats the piece:\n%.800s", where, out)
		}
		if got := take(); got != "" {
			t.Errorf("%s: another host was contacted with the piece:\n%s", where, got)
		}
	}
	// The same shapes with values nobody holds reach the sink, so each refusal above is
	// the held key and not a broken sender or a kit that refuses the shape.
	const cleanA, cleanB = "Tf3Mw8Yd", "Gs5Vn2Jq"
	deliver(sink.URL + "/collect?a=" + cleanA + "&b=" + cleanB)
	if got := take(); !strings.Contains(got, "a="+cleanA+"&b="+cleanB) {
		t.Errorf("deliver with clean query fields did not arrive: %q", got)
	}
	deliver(sink.URL+"/collect", "KEEPER_USER_AGENT="+cleanA)
	if got := take(); !strings.Contains(got, "ua="+cleanA) {
		t.Errorf("deliver with a clean User-Agent did not arrive: %q", got)
	}
	feedback(sink.URL + "/collect/" + cleanA + "/" + cleanB)
	if got := take(); !strings.Contains(got, cleanA+"/"+cleanB) {
		t.Errorf("feedback with a clean path did not arrive: %q", got)
	}
}

// commanderFake models Commander's one global managed-company context
// (commands/msp.py current_mc_id; commanderSession in internal/keeper/context_test.go),
// with a slow read inside a company. Any command that arrives while a read is in flight,
// and any switch into a company while another is selected, is an interleaving.
type commanderFake struct {
	mu         sync.Mutex
	current    string
	reading    string
	reads      map[string]int
	violations []string
}

func (f *commanderFake) handler(slowRead time.Duration) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		cmd := commandOf(body)
		fields := strings.Fields(cmd)
		w.Header().Set("Content-Type", "application/json")
		fail := func(msg string) {
			w.WriteHeader(http.StatusInternalServerError)
			fmt.Fprintf(w, `{"status":"error","error":%s}`, jsonStr("Unexpected error: "+msg))
		}
		if len(fields) == 0 {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		f.mu.Lock()
		if f.reading != "" {
			f.violations = append(f.violations, fmt.Sprintf("%q arrived while a read in company %s was in flight", cmd, f.reading))
		}
		switch fields[0] {
		case "switch-to-msp":
			in := f.current
			f.current = ""
			f.mu.Unlock()
			if in == "" {
				fail("switch-to-msp: Already MSP")
				return
			}
			fmt.Fprint(w, `{"status":"success","command":"switch-to-msp","message":"Switched back to MSP","data":null}`)
		case "switch-to-mc":
			in := f.current
			if in != "" && in != fields[1] {
				f.violations = append(f.violations, fmt.Sprintf("switch into company %s while company %s is selected", fields[1], in))
			}
			if in == "" {
				f.current = fields[1]
			}
			f.mu.Unlock()
			if in != "" {
				fail("switch-to-mc: Already switched to Managed Company id=" + in)
				return
			}
			fmt.Fprintf(w, `{"status":"success","command":"switch-to-mc","message":%s,"data":null}`, jsonStr("Switched to MC 'Company "+fields[1]+"'"))
		case "enterprise-info":
			company := f.current
			f.reading = "MSP"
			if company != "" {
				f.reading = company
			}
			if f.reads == nil {
				f.reads = map[string]int{}
			}
			f.reads[f.reading]++
			name := f.reading
			f.mu.Unlock()
			time.Sleep(slowRead)
			f.mu.Lock()
			f.reading = ""
			f.mu.Unlock()
			fmt.Fprint(w, report(cmd, fmt.Sprintf(`{"user_id":1,"email":"person@tenant%s.example","name":%s,"status":"Active"}`, name, jsonStr("Tenant-"+name+"-Person"))))
		default:
			f.mu.Unlock()
			w.WriteHeader(http.StatusForbidden)
		}
	})
}

// Class cross-tenant-context-race. Two processes with different --home, same user,
// against one Service Mode whose context is global: both read in turn. Each process's
// output names only its own company, and the fake sees no switch-in or other command
// while another company's read is in flight.
func TestClass_cross_tenant_context_race(t *testing.T) {
	t.Parallel()
	f := &commanderFake{}
	srv := tlsServer(t, f.handler(400*time.Millisecond))
	e := newEnv(t, &server{url: srv.URL, caFile: pemFile(t, srv)}, true)
	homeA, homeB := filepath.Join(e.home, "tenant-a"), filepath.Join(e.home, "tenant-b")
	for _, h := range []string{homeA, homeB} {
		if err := os.MkdirAll(h, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	type result struct {
		mc, out string
		err     error
	}
	const rounds = 3
	for round := 0; round < rounds; round++ {
		results := make(chan result, 2)
		for mc, home := range map[string]string{"10": homeA, "20": homeB} {
			go func() {
				cmd := exec.Command(bbCLI, "users", "list", "--mc", mc, "--home", home, "--json")
				cmd.Env, cmd.Dir = e.env, e.home
				out, err := cmd.CombinedOutput()
				results <- result{mc, string(out), err}
			}()
		}
		for i := 0; i < 2; i++ {
			r := <-results
			if r.err != nil {
				t.Errorf("round %d, company %s: %v\n%.1000s", round, r.mc, r.err, r.out)
			}
			other := "20"
			if r.mc == "20" {
				other = "10"
			}
			if !strings.Contains(r.out, "Tenant-"+r.mc+"-Person") {
				t.Errorf("round %d, company %s: the output does not name its own company:\n%.1000s", round, r.mc, r.out)
			}
			if strings.Contains(r.out, "Tenant-"+other+"-Person") || strings.Contains(r.out, "Tenant-MSP-Person") {
				t.Errorf("round %d, company %s: the output names another tenant:\n%.1000s", round, r.mc, r.out)
			}
		}
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.reads["10"] != rounds || f.reads["20"] != rounds {
		t.Errorf("want %d reads in each company, got %v", rounds, f.reads)
	}
	for _, v := range f.violations {
		t.Errorf("the two processes interleaved: %s", v)
	}
	if f.current != "" {
		t.Errorf("the session was left in company %s", f.current)
	}
}

// learnEvents is the number of rows in every learn_events table under home.
func learnEvents(t *testing.T, home string) int {
	t.Helper()
	total := 0
	_ = filepath.WalkDir(home, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(d.Name(), ".db") {
			return nil
		}
		db, err := sql.Open("sqlite", path)
		if err != nil {
			t.Fatalf("%s: %v", path, err)
		}
		defer db.Close()
		var has, n int
		if err := db.QueryRow(`SELECT count(*) FROM sqlite_master WHERE type='table' AND name='learn_events'`).Scan(&has); err != nil || has == 0 {
			return nil
		}
		if err := db.QueryRow(`SELECT count(*) FROM learn_events`).Scan(&n); err != nil {
			t.Fatalf("%s: %v", path, err)
		}
		total += n
		return nil
	})
	return total
}

// journalLines is the number of lines in the learn journal under home.
func journalLines(t *testing.T, home string) int {
	t.Helper()
	total := 0
	_ = filepath.WalkDir(home, func(path string, d fs.DirEntry, err error) error {
		if err == nil && !d.IsDir() && strings.HasPrefix(d.Name(), "journal-") && strings.HasSuffix(d.Name(), ".jsonl") {
			b, rerr := os.ReadFile(path)
			if rerr != nil {
				t.Fatalf("%s: %v", path, rerr)
			}
			total += bytes.Count(b, []byte("\n"))
		}
		return nil
	})
	return total
}

// mcpTools is the tools/list answer.
func mcpTools(t *testing.T, e *bbEnv) []struct {
	Name        string `json:"name"`
	Annotations struct {
		ReadOnlyHint *bool `json:"readOnlyHint"`
	} `json:"annotations"`
	InputSchema struct {
		Properties map[string]any `json:"properties"`
	} `json:"inputSchema"`
} {
	t.Helper()
	out := e.mcpRaw(t, []map[string]any{{"jsonrpc": "2.0", "id": 77, "method": "tools/list", "params": map[string]any{}}})
	for _, line := range strings.Split(out, "\n") {
		var msg struct {
			ID     int `json:"id"`
			Result struct {
				Tools []struct {
					Name        string `json:"name"`
					Annotations struct {
						ReadOnlyHint *bool `json:"readOnlyHint"`
					} `json:"annotations"`
					InputSchema struct {
						Properties map[string]any `json:"properties"`
					} `json:"inputSchema"`
				} `json:"tools"`
			} `json:"result"`
		}
		if json.Unmarshal([]byte(line), &msg) == nil && msg.ID == 77 {
			return msg.Result.Tools
		}
	}
	t.Fatalf("no tools/list answer:\n%.1500s", out)
	return nil
}

// Class write-without-confirm. recall is a read-only tool: through the MCP server it adds
// no learn_events row and no journal line, while the same recall on the CLI records both.
// And every MCP tool that does not declare itself read-only takes a confirm property.
func TestClass_write_without_confirm(t *testing.T) {
	t.Parallel()
	sv := echoServer(t)
	sv.honest.Store(true)
	e := newEnv(t, sv, true)
	recall := []map[string]any{{"name": "recall", "arguments": map[string]any{"query": "find the users"}}}
	first := e.mcpCalls(t, recall)
	if !strings.Contains(first, `found`) {
		t.Fatalf("the MCP recall did not answer; the test did not run:\n%.1500s", first)
	}
	if n := learnEvents(t, e.home); n != 0 {
		t.Errorf("an MCP recall added %d learn_events row(s)", n)
	}
	if n := journalLines(t, e.home); n != 0 {
		t.Errorf("an MCP recall added %d journal line(s)", n)
	}
	e.run(t, "recall", "find the users", "--json")
	events, lines := learnEvents(t, e.home), journalLines(t, e.home)
	if events == 0 {
		t.Errorf("the CLI recall recorded no learn_events row, so the checks above prove nothing")
	}
	if lines == 0 {
		t.Errorf("the CLI recall wrote no journal line, so the checks above prove nothing")
	}
	e.mcpCalls(t, recall)
	if n := learnEvents(t, e.home); n != events {
		t.Errorf("an MCP recall after the CLI's added rows: %d, then %d", events, n)
	}
	if n := journalLines(t, e.home); n != lines {
		t.Errorf("an MCP recall after the CLI's added journal lines: %d, then %d", lines, n)
	}
	tools := mcpTools(t, e)
	writable := 0
	for _, tool := range tools {
		if tool.Annotations.ReadOnlyHint != nil && *tool.Annotations.ReadOnlyHint {
			continue
		}
		writable++
		if _, ok := tool.InputSchema.Properties["confirm"]; !ok {
			t.Errorf("MCP tool %s is not readOnlyHint and has no confirm property", tool.Name)
		}
	}
	if len(tools) < 20 || writable == 0 {
		t.Errorf("tools/list gave %d tools, %d of them not read-only; the check is not reading the list", len(tools), writable)
	}
}

// Class startup-ordering. A credential inside a malformed config file is held before the
// first byte of output: it never appears in the startup output of either binary, a
// parse error included, and a parse error that would repeat it is withheld whole.
func TestClass_startup_ordering(t *testing.T) {
	t.Parallel()
	const secret = "Cf7Qx2L" + "m9Vp4Tz8Rk3Wn"
	crash := regexp.MustCompile(`panic:|goroutine \d+`)
	for name, content := range map[string]string{
		"unterminated string": "auth_header = \"Bearer " + secret + "\n",
		"bare line":           secret + "\n",
	} {
		t.Run(name, func(t *testing.T) {
			sv := echoServer(t)
			sv.honest.Store(true)
			e := newEnv(t, sv, true)
			named := filepath.Join(e.home, "named.toml")
			writeFile(t, named, content)
			writeFile(t, filepath.Join(configDir(t, e), "config.toml"), content)
			legacy := e.with("XDG_CONFIG_HOME=" + filepath.Join(e.home, "moved")) // the legacy path, by fallback
			for how, env := range map[string]*bbEnv{"default": e, "legacy": legacy, "env": e.with("KEEPER_CONFIG=" + named)} {
				outs := map[string]string{
					"version":       env.run(t, "version"),
					"--bogus-flag":  env.run(t, "--bogus-flag"),
					"command":       env.run(t, "companies", "list", "--json"),
					"mcp startup":   env.mcpCalls(t, nil),
					"mcp tool call": env.mcpCalls(t, []map[string]any{{"name": "companies_list", "arguments": map[string]any{}}}),
				}
				bogus := exec.Command(bbMCP, "--bogus-flag")
				bogus.Env, bogus.Dir = env.env, env.home
				b, _ := bogus.CombinedOutput()
				outs["mcp --bogus-flag"] = string(b)
				for what, out := range outs {
					noPiece(t, how+" "+what, out, secret)
					if m := crash.FindString(out); m != "" {
						t.Errorf("%s %s: the program crashed on the file (%s):\n%.1200s", how, what, m, out)
					}
				}
				// A flag-parse error repeats what it was given, so it proves the hold came
				// first. The same error for a value nobody holds prints in clear.
				if out := env.run(t, "--bogus-Xk4Pz9Wd2Hb7"); !strings.Contains(out, "Xk4Pz9Wd2Hb7") {
					t.Errorf("%s: the control parse error did not repeat its flag:\n%.800s", how, out)
				}
				out := env.run(t, "--bogus-"+secret)
				noPiece(t, how+" CLI parse error", out, secret)
				if !strings.Contains(out, bbWithheld) {
					t.Errorf("%s: the parse error lacks the kit's notice:\n%.800s", how, out)
				}
				mcpFlag := exec.Command(bbMCP, "--"+secret)
				mcpFlag.Env, mcpFlag.Dir = env.env, env.home
				b, _ = mcpFlag.CombinedOutput()
				noPiece(t, how+" MCP parse error", string(b), secret)
				if !strings.Contains(string(b), bbWithheld) {
					t.Errorf("%s: the MCP parse error lacks the kit's notice:\n%.800s", how, b)
				}
				control := exec.Command(bbMCP, "--bogus-Xk4Pz9Wd2Hb7")
				control.Env, control.Dir = env.env, env.home
				if b, _ = control.CombinedOutput(); !strings.Contains(string(b), "Xk4Pz9Wd2Hb7") {
					t.Errorf("%s: the control MCP parse error did not repeat its flag:\n%.800s", how, b)
				}
			}
			noPiece(t, "--config startup", e.run(t, "version", "--config", named), secret)
		})
	}
}

// Class profile-store. A value under api-key, under base-url (a URL with a password) and
// under a duplicate token key, first member and second, in profiles.json is held at
// startup: a report that repeats it is withheld. A saved --limit beside them is not held,
// so ordinary output still prints.
func TestClass_profile_store(t *testing.T) {
	t.Parallel()
	const (
		apiKey   = "Pk9Wd3T" + "n7Lx2Bv5Hm8Qr"
		password = "Ur4Zs8Y" + "c1Fg6Jn3Ka7Ep"
		firstDup = "Dn2Xb6Rt9Mq4Gw7Sv1Lc"
		lastDup  = "Vj5Hy3Cp8Tk2Nz6Fd9Qb"
	)
	sv := echoServer(t)
	sv.honest.Store(true)
	e := newEnv(t, sv, true)
	writeFile(t, filepath.Join(configDir(t, e), "profiles.json"), `{"profiles":{"p1":{"name":"p1","values":{`+
		`"limit":"7","api-key":"`+apiKey+`","base-url":"https://u:`+password+`@hooks.example/x",`+
		`"token":"`+firstDup+`","token":"`+lastDup+`"}}}}`)
	cli, mcp := readCompanies(t, e)
	for where, out := range map[string]string{"CLI": cli, "MCP": mcp} {
		if !strings.Contains(out, "HQ-Record") || strings.Contains(out, bbWithheld) {
			t.Errorf("%s: a profile store withheld ordinary output:\n%.1200s", where, out)
		}
	}
	for name, value := range map[string]string{"api-key": apiKey, "base-url password": password, "first token": firstDup, "second token": lastDup} {
		sv.show(value)
		cli, mcp := readCompanies(t, e)
		for where, out := range map[string]string{"CLI": cli, "MCP": mcp} {
			if strings.Contains(out, value) {
				t.Errorf("%s repeats the profile's %s:\n%.1200s", where, name, out)
			}
			if !strings.Contains(out, bbWithheld) {
				t.Errorf("%s: the profile's %s was not held (no notice):\n%.1200s", where, name, out)
			}
		}
	}
}

// Class explicit-config-origin. The run's base URL comes from the config it really reads.
// A default config.toml naming host A is only consulted, so with --config or KEEPER_CONFIG
// naming host B the key from B reaches B and never A; and a config with no base_url means
// the default origin, not A.
func TestClass_explicit_config_origin(t *testing.T) {
	t.Parallel()
	const keyB = "Bk3Rw7J" + "f2Ht9Ms5Zy4Pn"
	hostA, hostB := echoServer(t), echoServer(t)
	hostA.honest.Store(true)
	hostB.honest.Store(true)
	e := newEnv(t, hostB, false).without("KEEPER_BASE_URL")
	writeFile(t, filepath.Join(configDir(t, e), "config.toml"), fmt.Sprintf("base_url = %q\n", hostA.url))
	named := filepath.Join(e.home, "named.toml")
	writeFile(t, named, fmt.Sprintf("base_url = %q\napi_key = %q\n", hostB.url, keyB))
	for how, out := range map[string]string{
		"--config":          e.run(t, "companies", "list", "--json", "--config", named),
		"KEEPER_CONFIG":     e.with("KEEPER_CONFIG="+named).run(t, "companies", "list", "--json"),
		"KEEPER_CONFIG MCP": e.with("KEEPER_CONFIG="+named).mcpCalls(t, []map[string]any{{"name": "companies_list", "arguments": map[string]any{}}}),
	} {
		if !strings.Contains(out, "HQ-Record") || strings.Contains(out, bbWithheld) {
			t.Errorf("%s: the named config's base was refused:\n%.1200s", how, out)
		}
	}
	if !hostB.sawKey(keyB) {
		t.Errorf("the key from the named config never reached the host it names")
	}
	aHits := func() int {
		hostA.mu.Lock()
		defer hostA.mu.Unlock()
		return len(hostA.methods)
	}
	if n := aHits(); n != 0 {
		t.Errorf("the host only the default config names was contacted %d time(s)", n)
	}

	// A config with no base_url: the default origin, https://localhost:9090. A TLS fake
	// answers there when the port is free; the kit must let the key go to it.
	noBase := filepath.Join(e.home, "nobase.toml")
	writeFile(t, noBase, fmt.Sprintf("api_key = %q\n", keyB))
	var mu sync.Mutex
	var atDefault []string
	listening := false
	if ln, err := net.Listen("tcp", "127.0.0.1:9090"); err == nil {
		listening = true
		def := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			mu.Lock()
			atDefault = append(atDefault, r.Header.Get("api-key"))
			mu.Unlock()
			w.WriteHeader(http.StatusForbidden)
		}))
		_ = def.Listener.Close()
		def.Listener = ln
		def.EnableHTTP2 = true
		def.StartTLS()
		t.Cleanup(def.Close)
	} else {
		t.Logf("port 9090 is busy, so only the refusal half runs: %v", err)
	}
	// The fake's certificate names example.com, not localhost.
	over := e.with("KEEPER_TLS_SERVER_NAME=example.com")
	for how, out := range map[string]string{
		"--config":      over.run(t, "companies", "list", "--json", "--config", noBase),
		"KEEPER_CONFIG": over.with("KEEPER_CONFIG="+noBase).run(t, "companies", "list", "--json"),
	} {
		if strings.Contains(out, "withheld from outbound request") {
			t.Errorf("%s: the key was refused at the default origin:\n%.1200s", how, out)
		}
		noPiece(t, how+" output", out, keyB)
	}
	if n := aHits(); n != 0 {
		t.Errorf("with no base_url in the named config, the default config's host was contacted %d time(s)", n)
	}
	mu.Lock()
	defer mu.Unlock()
	if listening && len(atDefault) != 2 {
		t.Errorf("want one request per run at the default origin, got %d", len(atDefault))
	}
	for _, k := range atDefault {
		if k != keyB {
			t.Errorf("the default origin got api-key %q", k)
		}
	}
}

// A key pasted on the auth set-token line, positional or dashed, is repeated by the
// flag parser before the command runs; the line itself is the source that holds it.
func TestClass_set_token_line(t *testing.T) {
	t.Parallel()
	const key = "Qm7Zt2V" + "b9Lp4Nx8Rc3Hd6Wf1"
	e := newEnv(t, echoServer(t), true)
	for name, args := range map[string][]string{
		"dashed":     {"auth", "set-token", "--" + key},
		"one dash":   {"auth", "set-token", "-" + key},
		"positional": {"auth", "set-token", key},
		"after flag": {"auth", "set-token", "--json", "--" + key},
	} {
		out := e.run(t, args...)
		noPiece(t, name, out, key)
		if !strings.Contains(out, bbWithheld) {
			t.Errorf("%s: no kit notice:\n%.800s", name, out)
		}
	}
}
