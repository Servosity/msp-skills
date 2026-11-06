// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package action1blackbox

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Codex r1 4.34.2 class credential split across output members (once the
// output-units-reassembled hand-fix, now the shared credential kit,
// credkit-wiring): a held ACTION1_CLIENT_ID typed into feedback in two members
// with a member between them reaches neither the output nor feedback.jsonl.
// The ID is the reviewed input's shape (two 8-character halves), low-entropy;
// with no api-key- prefix and no "@", holdClientID holds it whole, and each
// half is an 8-character piece of it.
func TestFeedbackSplitAcrossMembersWithheld(t *testing.T) {
	t.Parallel()
	sv := echoServer(t)
	e := newEnv(t, sv, credStored)
	e.env = append(e.env, "ACTION1_CLIENT_ID=fsplwkzzwqmkmqzk")
	out := e.run(t, "feedback", `{"a":"fsplwkzz","gap":"x","b":"wqmkmqzk"}`, "--json")
	if strings.Contains(out, "fsplwkzz") || strings.Contains(out, "wqmkmqzk") {
		t.Errorf("the output carries a half of the held client ID:\n%s", out)
	}
	var files int
	_ = filepath.WalkDir(e.home, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		data, rerr := os.ReadFile(path)
		if rerr != nil {
			return nil
		}
		if filepath.Base(path) == "feedback.jsonl" {
			files++
		}
		if s := string(data); strings.Contains(s, "fsplwkzz") || strings.Contains(s, "wqmkmqzk") {
			t.Errorf("%s carries a half of the held client ID:\n%s", path, s)
		}
		return nil
	})
	if files == 0 {
		t.Fatalf("feedback wrote no feedback.jsonl; the test did not run:\n%s", out)
	}
}

// tripleEncoded percent-encodes every byte of s, then encodes the result's
// percent signs twice more: "K" becomes "%25254B".
func tripleEncoded(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		fmt.Fprintf(&b, "%%2525%02X", s[i])
	}
	return b.String()
}

// The encoded variant of the class above (gradient's open class 1, moved here
// from internal/credwire): with ACTION1_CLIENT_SECRET set, feedback text
// carrying the secret's two halves, each triple percent-encoded, in two members
// with a member between them must print neither half, in either spelling, and
// write neither to feedback.jsonl or any other file. Each half is an
// 8-character piece of the held secret, which the kit finds after
// percent-decoding at every depth. A value of the same shape that is not held is
// the control: it must reach the output and feedback.jsonl, so the run proves
// it reached both sinks.
func TestFeedbackEncodedHalvesWithheld(t *testing.T) {
	t.Parallel()
	const secret = "fbkeyzmmzqzzmmqz"
	const control = "fbctlzmmwmmqkwkz"
	feedback := func(value string) (string, map[string]string) {
		sv := echoServer(t)
		e := newEnv(t, sv, credStored)
		e.env = append(e.env, "ACTION1_CLIENT_SECRET="+secret)
		text := fmt.Sprintf(`{"a":%q,"gap":"x","b":%q}`, tripleEncoded(value[:8]), tripleEncoded(value[8:]))
		out := e.run(t, "feedback", text, "--json")
		files := map[string]string{}
		_ = filepath.WalkDir(e.home, func(path string, d fs.DirEntry, err error) error {
			if err == nil && !d.IsDir() {
				if b, rerr := os.ReadFile(path); rerr == nil {
					files[path] = string(b)
				}
			}
			return nil
		})
		return out, files
	}
	out, files := feedback(control)
	enc := tripleEncoded(control[:8])
	var ledger string
	for p, s := range files {
		if filepath.Base(p) == "feedback.jsonl" {
			ledger += s
		}
	}
	if !strings.Contains(out, enc) || !strings.Contains(ledger, enc) {
		t.Fatalf("control: an unheld value did not reach the output and feedback.jsonl, so this test proves nothing\noutput:\n%s\nfeedback.jsonl:\n%s", out, ledger)
	}
	out, files = feedback(secret)
	pieces := []string{secret[:8], secret[8:], tripleEncoded(secret[:8]), tripleEncoded(secret[8:])}
	files["output"] = out
	for where, text := range files {
		for _, p := range pieces {
			if strings.Contains(text, p) {
				t.Errorf("%s carries %q (a half of the held client secret):\n%s", where, p, text)
			}
		}
	}
}

// Codex r1 (action1 reprint on 4.34.2) class credential split across protocol
// messages, moved here from cmd/action1-mcp when the connector's own message
// writer was retired: two unknown-tool errors, each naming half of the held
// token, were each filtered alone and printed the token whole. Each half is an
// 8-character piece (or longer) of the held token, so the kit withholds each
// frame that carries one, and the frame that stands in for it still answers
// its request. The control session, with another token held, prints both halves.
func TestTokenSplitAcrossProtocolMessagesIsWithheld(t *testing.T) {
	t.Parallel()
	const first, second = "pspltzqmkwzz", "pssplqwkmzmm"
	session := func(token string) (string, map[int]string) {
		sv := echoServer(t)
		e := newEnv(t, sv, credEnvToken)
		e.env = append(withoutEnv(e.env, "ACTION1_OAUTH2"), "ACTION1_OAUTH2="+token)
		out := e.mcpPhases(t,
			[]map[string]any{{"name": first, "arguments": map[string]any{}}},
			[]map[string]any{{"name": second, "arguments": map[string]any{}}})
		return out, mcpReplies(out)
	}
	out, replies := session(bbToken)
	if !strings.Contains(replies[1], first) || !strings.Contains(replies[2], second) {
		t.Fatalf("control: the unknown-tool errors do not name the tool, so this test proves nothing:\n%.2000s", out)
	}
	out, replies = session(first + second)
	for _, half := range []string{first, second} {
		if strings.Contains(out, half) {
			t.Errorf("a half of the held token reached the client (%q):\n%.2000s", half, out)
		}
	}
	for id := 1; id <= 2; id++ {
		if !strings.Contains(replies[id], `"error"`) {
			t.Errorf("the withheld message must still answer request %d:\n%.2000s", id, out)
		}
	}
}
