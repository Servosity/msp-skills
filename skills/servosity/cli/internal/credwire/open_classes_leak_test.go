// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

// Hand-written (NOT generated; handfixes.json credkit-wiring): the credential
// containment inputs earlier reviews left open, each proved against output,
// the store, the events list and MCP through the real binaries.

package credwire

import (
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"net/url"
	"strings"
	"testing"
)

const (
	cleanA = `"aaaaaaaa-0000-4000-8000-00000000000a"`
	cleanB = `"bbbbbbbb-0000-4000-8000-00000000000b"`
	// No 8-character run in common with the clean ids: a held value counts in
	// pieces of 8 or more anywhere, so a shared run would withhold those too.
	poison = `"c7d1e9f2-5a3b-4c6d-9e8f-1a2b3c4d5e6f"`
	late   = `"dddddddd-0000-4000-8000-00000000000d"`
)

// watchSinks drives a baseline poll, then a poll that adds a poisoned issue
// and a clean one, and proves forms reached no sink while the clean issue
// still came through: the watcher skips the issue and keeps going.
func watchSinks(t *testing.T, s sandbox, f *fakeAPI, env []string, poisoned string, forms ...string) {
	t.Helper()
	cli, mcp := binaries(t)
	f.setIssues(issueJSON(cleanA, "11"), poisoned)
	res := run(t, cli, env, "issues", "watch", "--once", "--json")
	if res.code != 0 {
		t.Errorf("baseline poll failed (exit %d): %s", res.code, res.all())
	}
	requireAbsent(t, "issues watch (baseline)", res.all(), forms...)

	f.setIssues(issueJSON(cleanA, "11"), poisoned, issueJSON(late, "12"))
	res = run(t, cli, env, "issues", "watch", "--once", "--json")
	if res.code != 0 {
		t.Errorf("second poll failed (exit %d): %s", res.code, res.all())
	}
	requireAbsent(t, "issues watch", res.all(), forms...)

	if hits := s.filesHolding(t, forms...); len(hits) > 0 {
		t.Errorf("the credential reached the store: %v", hits)
	}
	res = run(t, cli, env, "events", "list", "--since", "0", "--json")
	requireAbsent(t, "events list", res.all(), forms...)
	if !strings.Contains(res.stdout, strings.Trim(late, `"`)) {
		t.Errorf("the clean issue added by the second poll has no event, so one poisoned issue stopped the watcher: %s", res.all())
	}
	frames, stderr := mcpCall(t, mcp, env, "events_list", map[string]any{"since": "0"})
	requireAbsent(t, "MCP events_list", frames+stderr, forms...)
}

// readSinks proves a server answer carrying forms reaches neither CLI output
// nor an MCP result, and that nothing kept it on disk.
func readSinks(t *testing.T, s sandbox, env []string, forms ...string) {
	t.Helper()
	cli, mcp := binaries(t)
	res := run(t, cli, env, "resellers", "issues", "resellers", "7", "--json", "--no-cache")
	requireAbsent(t, "resellers issues", res.all(), forms...)
	frames, stderr := mcpCall(t, mcp, env, "servosity-msp_read", map[string]any{
		"endpoint_id": "resellers.issues.resellers", "params": map[string]any{"id": "7"},
	})
	requireAbsent(t, "MCP servosity-msp_read", frames+stderr, forms...)
	if hits := s.filesHolding(t, forms...); len(hits) > 0 {
		t.Errorf("the credential reached a file: %v", hits)
	}
}

// operatorSinks proves operator text carrying form is saved or shown by no
// command that takes text: a profile, feedback, a teach note, a recall query.
func operatorSinks(t *testing.T, s sandbox, env []string, form string) {
	t.Helper()
	cli, _ := binaries(t)
	for _, args := range [][]string{
		{"profile", "save", "leak", "--json", "--description", form},
		{"feedback", form},
		{"teach", "--query", "find items", "--resource-type", "companies", "--resource", "1", "--notes", form},
		{"recall", form, "--json"},
	} {
		res := run(t, cli, env, args...)
		requireAbsent(t, strings.Join(args[:2], " "), res.all(), form)
		if hits := s.filesHolding(t, form); len(hits) > 0 {
			t.Errorf("%s persisted it: %v", strings.Join(args[:2], " "), hits)
		}
	}
	// The same profile with ordinary text saves, so the absence above is the
	// credential being withheld, not a command that keeps nothing.
	res := run(t, cli, env, "profile", "save", "plain", "--json", "--description", "an ordinary note")
	if hits := s.filesHolding(t, "an ordinary note"); len(hits) == 0 {
		t.Errorf("profile save keeps nothing even without a credential, so this test proved nothing: %s", res.all())
	}
}

// Round 2's input: a held 6-digit MFA code equal to an issue's integer field,
// under the header the API names and under any other [headers] name.
func TestHeldMFACodeEqualToAnIssueIntegerField(t *testing.T) {
	t.Parallel()
	const code = "482913"
	for _, header := range []string{"X-Servosity-Mfa", "X-Partner-Ref"} {
		t.Run(header, func(t *testing.T) {
			t.Parallel()
			s := newSandbox(t)
			f := newFakeAPI(t, s)
			env := s.apiEnv(t, f, fmt.Sprintf("[headers]\n%s = %q\n", header, code), syntheticToken)
			watchSinks(t, s, f, env, issueJSON(poison, code), code)
			f.setIssues(issueJSON(poison, code))
			readSinks(t, s, env, code)
		})
	}
}

// Round 2's second input: an id in another case. The held value is an issue
// id in upper case; the issue carries it in lower case.
func TestHeldValueEqualToAnIssueIDInAnotherCase(t *testing.T) {
	t.Parallel()
	id := strings.Trim(poison, `"`)
	for name, env := range map[string]func(s sandbox, f *fakeAPI) []string{
		"token": func(s sandbox, f *fakeAPI) []string {
			return s.apiEnv(t, f, "", strings.ToUpper(id))
		},
		"header": func(s sandbox, f *fakeAPI) []string {
			return s.apiEnv(t, f, fmt.Sprintf("[headers]\nX-Partner-Ref = %q\n", strings.ToUpper(id)), syntheticToken)
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			s := newSandbox(t)
			f := newFakeAPI(t, s)
			e := env(s, f)
			watchSinks(t, s, f, e, issueJSON(poison, "11"), id)
			f.setIssues(issueJSON(poison, "11"))
			readSinks(t, s, e, id)
		})
	}
}

// The last review's first input: a short held value in an encoded form.
func TestShortHeldValueInEncodedForms(t *testing.T) {
	t.Parallel()
	const code = "604813"
	pct := func(v string) string {
		var b strings.Builder
		for i := 0; i < len(v); i++ {
			fmt.Fprintf(&b, "%%%02x", v[i])
		}
		return b.String()
	}
	forms := map[string]string{
		"as written": code,
		"base64":     base64.StdEncoding.EncodeToString([]byte(code)),
		"hex":        hex.EncodeToString([]byte(code)),
		"percent":    pct(code),
		"url double": url.QueryEscape(pct(code)),
	}
	config := fmt.Sprintf("[headers]\nX-Servosity-Mfa = %q\n", code)
	for name, form := range forms {
		t.Run("operator text "+name, func(t *testing.T) {
			t.Parallel()
			s := newSandbox(t)
			f := newFakeAPI(t, s)
			operatorSinks(t, s, s.apiEnv(t, f, config, syntheticToken), form)
		})
		t.Run("server answer "+name, func(t *testing.T) {
			t.Parallel()
			s := newSandbox(t)
			f := newFakeAPI(t, s, fmt.Sprintf(`{"id":%s,"state":"ACTIVE","note":%q}`, poison, form))
			readSinks(t, s, s.apiEnv(t, f, config, syntheticToken), form)
		})
	}
	// The events list: the code's hex form is all digits, so it passes the id
	// pattern and is a stored field.
	t.Run("events list", func(t *testing.T) {
		t.Parallel()
		s := newSandbox(t)
		f := newFakeAPI(t, s)
		id := hex.EncodeToString([]byte(code))
		watchSinks(t, s, f, s.apiEnv(t, f, config, syntheticToken), issueJSON(id, "11"), id)
	})
}

// The last review's second input: a short held value with a separator in it,
// and inside a serialized record.
func TestShortHeldValueWithASeparator(t *testing.T) {
	t.Parallel()
	const value = "ab-cd"
	config := fmt.Sprintf("[headers]\nX-Auth-Token = %q\n", value)
	for name, form := range map[string]string{
		"as written":        value,
		"in a record":       `{"a":"` + value + `"}`,
		"in a shown record": "note=" + value + ";",
	} {
		t.Run("operator text "+name, func(t *testing.T) {
			t.Parallel()
			s := newSandbox(t)
			f := newFakeAPI(t, s)
			operatorSinks(t, s, s.apiEnv(t, f, config, syntheticToken), form)
		})
		t.Run("server answer "+name, func(t *testing.T) {
			t.Parallel()
			s := newSandbox(t)
			quoted := strings.ReplaceAll(form, `"`, `\"`)
			f := newFakeAPI(t, s, fmt.Sprintf(`{"id":%s,"state":"ACTIVE","note":"%s"}`, poison, quoted))
			readSinks(t, s, s.apiEnv(t, f, config, syntheticToken), value)
		})
	}
	// A value with a separator that a stored timestamp carries as a whole
	// token: split on the separator it matched nothing, and at 7 characters
	// no substring check saw it.
	t.Run("events list", func(t *testing.T) {
		t.Parallel()
		const stamp = "2031-07"
		s := newSandbox(t)
		f := newFakeAPI(t, s)
		poisoned := fmt.Sprintf(`{"id":%s,"state":"ACTIVE","company":11,"updated_at":"%s-04T00:00:00Z"}`, poison, stamp)
		env := s.apiEnv(t, f, fmt.Sprintf("[headers]\nX-Partner-Ref = %q\n", stamp), syntheticToken)
		watchSinks(t, s, f, env, poisoned, stamp)
	})
}

// The last review's third input: a parse error printed before the program
// holds anything, for a credential from the environment and from a config
// file, whole and short, from both binaries.
func TestParseErrorsPrintedBeforeTheProgramHoldsAnything(t *testing.T) {
	t.Parallel()
	cli, mcp := binaries(t)
	const short = "604813"
	cases := []struct {
		name   string
		config string
		token  string
		bin    string
		args   []string
		forms  []string
	}{
		{name: "cli unknown flag, token in env", token: syntheticToken, bin: cli,
			args: []string{"version", "--" + syntheticToken}, forms: []string{syntheticToken}},
		{name: "cli unknown command, token in env", token: syntheticToken, bin: cli,
			args: []string{syntheticToken}, forms: []string{syntheticToken}},
		{name: "cli unknown flag, base64 of the token", token: syntheticToken, bin: cli,
			args:  []string{"version", "--" + base64.RawURLEncoding.EncodeToString([]byte(syntheticToken))},
			forms: []string{base64.RawURLEncoding.EncodeToString([]byte(syntheticToken))}},
		{name: "cli unknown flag, short header value", config: "[headers]\nX-Servosity-Mfa = \"" + short + "\"\n", token: syntheticToken, bin: cli,
			args: []string{"version", "--" + short}, forms: []string{short}},
		{name: "cli unknown flag, auth_header in config", config: "auth_header = \"" + syntheticAuthHeader + "\"\n", bin: cli,
			args: []string{"version", "--" + syntheticAuthHeader}, forms: []string{syntheticAuthHeader}},
		{name: "cli unknown flag, config does not parse", config: "auth_header = \"" + syntheticAuthHeader + "\"\nbroken = [\n", bin: cli,
			args: []string{"version", "--" + syntheticAuthHeader}, forms: []string{syntheticAuthHeader}},
		{name: "mcp unknown flag, token in env", token: syntheticToken, bin: mcp,
			args: []string{"--" + syntheticToken}, forms: []string{syntheticToken}},
		{name: "mcp bad transport, token in env", token: syntheticToken, bin: mcp,
			args: []string{"--transport", syntheticToken}, forms: []string{syntheticToken}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			s := newSandbox(t)
			cfg := s.writePrivate(t, "sv.toml", c.config)
			env := s.env("SERVOSITY_MSP_CONFIG=" + cfg)
			if c.token != "" {
				env = append(env, "SERVOSITY_MSP_TOKEN="+c.token)
			}
			res := run(t, c.bin, env, c.args...)
			if res.code == 0 {
				t.Errorf("the input did not fail to parse, so this test proved nothing: %s", res.all())
			}
			requireAbsent(t, "the parse error", res.all(), c.forms...)
			if hits := s.filesHolding(t, c.forms...); len(hits) > 0 {
				t.Errorf("the parse error's journal kept it: %v", hits)
			}
		})
	}
}

// A mirrored MCP tool runs the CLI as a child, and a child never gets a
// credential in its argv or environment: the token still reaches the API, so
// the tool answers instead of being refused.
func TestMirroredToolAuthenticatesWithTheEnvironmentToken(t *testing.T) {
	t.Parallel()
	_, mcp := binaries(t)
	s := newSandbox(t)
	f := newFakeAPI(t, s)
	env := s.apiEnv(t, f, "", syntheticToken, "SERVOSITY_MSP_MCP_HTTP_TOKEN="+syntheticBearer)
	frames, stderr := mcpCall(t, mcp, env, "attention", map[string]any{"refresh": true})
	requireAbsent(t, "MCP attention", frames+stderr, syntheticToken, syntheticBearer)
	if !f.sawAuth("Token " + syntheticToken) {
		t.Errorf("the mirrored tool's child never sent the token; frames:\n%s\nstderr:\n%s", frames, stderr)
	}
}

// Assembled at run time so no literal looks like a live credential to a
// secret scanner.
var (
	syntheticToken      = "c0ffee11" + "d00d5eed" + "01234567" + "89abcdef" + "0badf00d"
	syntheticAuthHeader = "Token " + "feedface" + "c0ffee22" + "01234567"
	syntheticBearer     = "b16b00b5" + "deadc0de" + "76543210"
)
