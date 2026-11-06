// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package credwire_test

// The credential classes the press 4.34.2 reprint's last Codex round left
// open (cli-factory action1 LEDGER-NOTES section 7, classes 1-3), closed
// through the shared credential kit (handfixes.json credkit-wiring), and two
// Action1 data shapes the kit must not withhold. Each input is the one the
// review named, on the built binaries (ACTION1_LEAK_BIN_DIR runs them on
// binaries built elsewhere).

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"action1-pp-cli/internal/credkit"
)

// reply is the JSON-RPC message in out that answers request id, or "".
func reply(out string, id int) string {
	for _, l := range strings.Split(out, "\n") {
		var m struct {
			ID *int `json:"id"`
		}
		if json.Unmarshal([]byte(l), &m) == nil && m.ID != nil && *m.ID == id {
			return l
		}
	}
	return ""
}

func call(name string, args map[string]any) rpc {
	return rpc{"tools/call", map[string]any{"name": name, "arguments": args}}
}

// orgPage is an Action1 organizations page whose one record carries note.
func orgPage(note string) string {
	return fmt.Sprintf(`{"items":[{"id":"org-1042","name":"HQ-Record","note":%q}],"total_items":1,"next_page":""}`, note)
}

// Open class 1, "credential persisted in journal": the journal filled its
// timestamp after the entry was checked, so a client secret equal to today's
// date (or that date and the T that follows it) reached the journal. The kit
// checks the record as written. The control, an unrelated secret, proves the
// run journals an entry carrying today's date.
func TestJournalTimestampEqualToCredentialNotPersisted(t *testing.T) {
	cliBin, _ := binaries(t)
	// run journals one version call with secret(today's UTC date) held; it
	// retries once when UTC midnight passed during the run, so the date the
	// secret repeats is the date the journal wrote.
	run := func(t *testing.T, secret func(day string) string) (held, out string, files map[string]string) {
		t.Helper()
		for try := 0; ; try++ {
			day := time.Now().UTC().Format("2006-01-02")
			home := t.TempDir()
			held = secret(day)
			stdout, stderr, _ := runSplit(t, cliBin, closedEnv(home, "ACTION1_CLIENT_SECRET="+held), "version")
			if day == time.Now().UTC().Format("2006-01-02") || try > 0 {
				return held, stdout + stderr, filesUnder(t, home)
			}
		}
	}
	_, _, files := run(t, func(string) string { return "jtctl-kqzmwkqzmw" })
	today := time.Now().UTC().Format("2006-01-02")
	var journaled bool
	for _, text := range files {
		journaled = journaled || strings.Contains(text, `"ts":"`+today)
	}
	if !journaled {
		t.Fatalf("control: version journaled no entry dated today, so this test proves nothing: %v", files)
	}
	for name, secret := range map[string]func(string) string{
		"the date":           func(day string) string { return day },
		"the date and its T": func(day string) string { return day + "T" },
	} {
		t.Run(name, func(t *testing.T) {
			held, out, files := run(t, secret)
			if strings.Contains(out, held) {
				t.Errorf("the output carries the held client secret %q:\n%s", held, out)
			}
			for path, text := range files {
				if strings.Contains(text, held) {
					t.Errorf("%s carries the held client secret %q:\n%s", path, held, text)
				}
			}
		})
	}
}

// Open class 2, "short credential in output": a held client secret of three
// characters printed in partial and decoded spellings (quoted-printable
// =5Aq9, base64 eFpxOQ== of xZq9). Each reading runs on its own, so one
// reading the kit withholds cannot hide another it misses. The record carries
// the spelling in a member; the control, the same record with an unrelated
// secret held, prints it, so each case proves the spelling reaches stdout.
func TestShortCredentialInDecodedView(t *testing.T) {
	cliBin, _ := binaries(t)
	const short = "Zq9"
	for _, v := range []struct{ name, text string }{
		{"quoted-printable", "=5Aq9"},
		{"base64 of a longer run", "eFpxOQ=="},
		{"hex bytes", "0x5a 0x71 0x39"},
		{"character codes", "90, 113, 57"},
		{"rot13", "Md9"},
	} {
		t.Run(v.name, func(t *testing.T) {
			base, ca := fakeAPI(t, orgPage(v.text))
			run := func(secret string) (string, int) {
				home := t.TempDir()
				env := closedEnv(home, "ACTION1_BASE_URL="+base, "ACTION1_CA_CERT="+ca,
					"ACTION1_OAUTH2=sctok-mqzkwmqzkwmqzk", "ACTION1_CLIENT_SECRET="+secret)
				out, errOut, code := runSplit(t, cliBin, env, "organizations", "get", "--json")
				return out + errOut, code
			}
			if out, _ := run("scctl-wqkzmqwkzq"); !strings.Contains(out, v.text) {
				t.Fatalf("control: %q did not reach the output, so this case proves nothing:\n%s", v.text, out)
			}
			out, code := run(short)
			if code != credkit.ExitRefused && !strings.Contains(out, "HQ-Record") && !strings.Contains(out, credkit.Notice("stdout")) {
				t.Fatalf("the run printed neither the record nor the kit's notice, so it proves nothing:\n%s", out)
			}
			if strings.Contains(out, v.text) {
				t.Errorf("%q (%s of the held %q) reached the output:\n%s", v.text, v.name, short, out)
			}
		})
	}
}

// Open class 3, "credential split across protocol messages": two unknown-tool
// errors each name half of the held token, with a call between them. In the
// review's input the middle call newly holds the first half (a password
// argument to context), which reset the delivery history, so each half printed
// and the client read the token whole. A middle call whose response newly
// holds another value, and 64 KiB between the halves, are the same class. The
// kit withholds each frame that carries an 8-character piece of a held value,
// so neither half prints, and the frame that stands in for one still answers
// its request. The control, another token held, prints both halves.
func TestCredentialSplitAcrossMCPMessages(t *testing.T) {
	_, mcpBin := binaries(t)
	// The reviewed token's shape (24 characters, two 12-character halves) in a
	// low-entropy spelling.
	const first, second = "mspakqzwmkqz", "mspbwzqkmzwq"
	read := call("action1_read", map[string]any{"endpoint_id": "organizations.get"})
	for _, c := range []struct {
		name, body string
		middle     rpc
	}{
		{"the review's input: the middle call newly holds the first half", orgPage("HQ-Record"), call("context", map[string]any{"password": first})},
		{"the middle response newly holds another value", `{"items":[{"id":"org-1042","name":"HQ-Record","password":"mspnewheldqzkwmz"}],"total_items":1,"next_page":""}`, read},
		{"64 KiB between the halves", orgPage(strings.Repeat("HQ-Padding ", 7000)), read},
	} {
		t.Run(c.name, func(t *testing.T) {
			base, ca := fakeAPI(t, c.body)
			session := func(token string) string {
				env := closedEnv(t.TempDir(), "ACTION1_BASE_URL="+base, "ACTION1_CA_CERT="+ca, "ACTION1_OAUTH2="+token)
				out, _, _ := mcpRun(t, mcpBin, env, call(first, map[string]any{}), c.middle, call(second, map[string]any{}))
				return out
			}
			out := session("mspctlkwqzmkwqzmkwqzmkwq")
			if !strings.Contains(reply(out, 2), first) || !strings.Contains(reply(out, 4), second) || reply(out, 3) == "" {
				t.Fatalf("control: the unknown-tool errors do not name the tool, or the middle call went unanswered, so this case proves nothing:\n%.3000s", out)
			}
			out = session(first + second)
			for _, half := range []string{first, second} {
				if strings.Contains(out, half) {
					t.Errorf("a half of the held token reached the client (%q):\n%.3000s", half, out)
				}
			}
			for _, id := range []int{2, 4} {
				if reply(out, id) == "" {
					t.Errorf("request %d went unanswered: a withheld message must still answer it:\n%.3000s", id, out)
				}
			}
		})
	}
}

// Action1 issues the OAuth client ID as api-key-<uuid>@action1.com. Held whole,
// its 8-character pieces would withhold every line naming action1.com, the API
// host included, so the client ID is an identifier and holdClientID holds only
// the unique part. From the environment and from config.toml: the tool list
// and a dry run naming the API host print whole, and a response that repeats
// the unique part is withheld.
func TestClientIDDoesNotWithholdTheAPIHost(t *testing.T) {
	cliBin, mcpBin := binaries(t)
	const unique = "c1d2c1d2-c1d2-c1d2-c1d2-c1d2c1d2c1d2"
	const id, secret = "api-key-" + unique + "@action1.com", "cidsec-kwzqmkwzqm"
	for _, src := range []string{"environment", "config.toml"} {
		t.Run(src, func(t *testing.T) {
			home := t.TempDir()
			var env []string
			if src == "environment" {
				env = closedEnv(home, "ACTION1_CLIENT_ID="+id, "ACTION1_CLIENT_SECRET="+secret)
			} else {
				writeConfig(t, home, fmt.Sprintf("client_id = %q\nclient_secret = %q\n", id, secret), false)
				env = closedEnv(home)
			}
			out, errOut, _ := runSplit(t, cliBin, env, "organizations", "get", "--dry-run")
			if !strings.Contains(out+errOut, "https://app.action1.com/api/3.0/organizations") || strings.Contains(out+errOut, "credkit: withheld") {
				t.Errorf("the dry run did not name the API host whole:\n%s%s", out, errOut)
			}
			replies, mcpErr, _ := mcpRun(t, mcpBin, env, rpc{"tools/list", map[string]any{}})
			if list := reply(replies, 2); !strings.Contains(list, `"tools":[`) || !strings.Contains(list, "action1_read") || strings.Contains(replies+mcpErr, "credkit: withheld") {
				t.Errorf("tools/list did not answer whole:\n%.2000s\nstderr:\n%s", replies, mcpErr)
			}
			base, ca := fakeAPIFunc(t, func(_ http.Header, r *http.Request) string {
				if strings.HasSuffix(r.URL.Path, "/oauth2/token") {
					return `{"access_token":"cidacc-mzkqwmzkqw","refresh_token":"cidref-qwkzmqwkzm","expires_in":3600,"token_type":"Bearer"}`
				}
				return orgPage("issued to " + unique)
			})
			live := append(append([]string{}, env...), "ACTION1_BASE_URL="+base, "ACTION1_CA_CERT="+ca)
			out, errOut, _ = runSplit(t, cliBin, live, "organizations", "get", "--json")
			if strings.Contains(out+errOut, unique) || !strings.Contains(out+errOut, credkit.Notice("stdout")) {
				t.Errorf("a response repeating the client ID's unique part was not withheld:\n%s%s", out, errOut)
			}
		})
	}
}

// remote_session (the browser connection URL), CVSS authentication,
// session_timeout and token_type are Action1 data, not credentials: a remote
// session record reaches the operator whole. The same record carrying the held
// token is withheld, so the case proves the kit read it.
func TestRemoteSessionURLReachesTheOperator(t *testing.T) {
	cliBin, _ := binaries(t)
	const token = "rstok-qzkwmqzkwmqzkw"
	const sessionURL = "https://remote.action1.com/remote?id=c0c0c0c0-d0d0-4e0e-8f0f-a0a0a0a0a0a0"
	record := func(url string) string {
		return fmt.Sprintf(`{"id":"rs-7731","type":"RemoteSession","connected":"yes","connection_type":"assistance","endpoint_name":"HQ-Desk","remote_session":%q,"authentication":"none","session_timeout":"3600","token_type":"assistance"}`, url)
	}
	get := func(body string) string {
		base, ca := fakeAPI(t, body)
		env := closedEnv(t.TempDir(), "ACTION1_BASE_URL="+base, "ACTION1_CA_CERT="+ca, "ACTION1_OAUTH2="+token)
		out, errOut, _ := runSplit(t, cliBin, env, "endpoints", "managed-id-remote-sessions-session-id-get", "org-1042", "ep-7731", "rs-7731", "--json")
		return out + errOut
	}
	out := get(record(sessionURL))
	for _, want := range []string{sessionURL, `"authentication"`, `"session_timeout"`, `"token_type"`} {
		if !strings.Contains(out, want) {
			t.Errorf("the remote session record lost %s:\n%s", want, out)
		}
	}
	if strings.Contains(out, "credkit: withheld") {
		t.Errorf("the remote session record was withheld:\n%s", out)
	}
	if out := get(record(sessionURL + "~" + token)); strings.Contains(out, token) || !strings.Contains(out, credkit.Notice("stdout")) {
		t.Errorf("a record carrying the held token was not withheld:\n%s", out)
	}
}
