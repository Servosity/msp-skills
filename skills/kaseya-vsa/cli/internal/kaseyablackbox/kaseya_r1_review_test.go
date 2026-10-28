// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package kaseyablackbox

// Review round 1 (Codex, 2026-10-28): the three [P1] repros, driven through
// the built CLI and MCP binaries exactly as reported.
//
//   - P1-A: a record whose ID repeats the credential was stored with that ID
//     (resources.id, resources_fts.id, the typed table).
//   - P1-B: a response name holding the ROT13 of the credential (an encoding
//     the client's own masker knows) passed the output chokepoint unchanged:
//     CLI JSON, MCP results and --deliver webhook.
//   - P1-C: an unknown flag spelled as the credential reached the learn
//     journal, and feedback text reached feedback.jsonl and the feedback
//     endpoint.
//
// The oracle is the exact set of strings the test sent, matched without
// regard to case, so a pass does not depend on the program's own decoder.

import (
	"bytes"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

const (
	rvTokenID     = "kvsaid" + "4d2a9c71" // split: a synthetic fixture, not a credential for secret scanners
	rvTokenSecret = "Secret" + "Alpha901" // the review's own example value
)

func rvRot13(s string) string {
	return strings.Map(func(r rune) rune {
		switch {
		case r >= 'a' && r <= 'z':
			return 'a' + (r-'a'+13)%26
		case r >= 'A' && r <= 'Z':
			return 'A' + (r-'A'+13)%26
		}
		return r
	}, s)
}

func rvFullwidth(s string) string {
	return strings.Map(func(r rune) rune {
		if r >= 0x21 && r <= 0x7E {
			return r + 0xFEE0
		}
		return r
	}, s)
}

func rvPercentU(s string) string {
	var b strings.Builder
	for _, r := range s {
		fmt.Fprintf(&b, "%%u%04X", r)
	}
	return b.String()
}

// rvForms are encodings the client masker (internal/client
// maskCredentialText) recognizes and the round-0 chokepoint did not.
func rvForms() map[string]string {
	return map[string]string{
		"rot13":     rvRot13(rvTokenSecret),
		"upper":     strings.ToUpper(rvTokenSecret),
		"fullwidth": rvFullwidth(rvTokenSecret),
		"percent-u": rvPercentU(rvTokenSecret),
	}
}

// rvOracle is every string that must never be written or sent, lowercased.
func rvOracle() []string {
	seen := map[string]bool{}
	var out []string
	for _, f := range append([]string{rvTokenSecret}, rvFormList()...) {
		if low := strings.ToLower(f); !seen[low] {
			seen[low] = true
			out = append(out, low)
		}
	}
	return out
}

func rvFormList() []string {
	forms := rvForms()
	return []string{forms["rot13"], forms["upper"], forms["fullwidth"], forms["percent-u"]}
}

func rvAssertNone(t *testing.T, where string, data []byte) {
	t.Helper()
	low := bytes.ToLower(data)
	for _, f := range rvOracle() {
		if bytes.Contains(low, []byte(f)) {
			t.Errorf("%s holds the credential as %q", where, f)
		}
	}
}

// rvServer answers every list with three organizations: one whose ID is the
// credential, one whose name is each client-masker encoding of it, and an
// ordinary one that must survive.
func rvServer(t *testing.T) string {
	t.Helper()
	var items []string
	items = append(items, fmt.Sprintf(`{"Id":%q,"Name":"Acme"}`, rvTokenSecret))
	i := 10
	for _, k := range []string{"rot13", "upper", "fullwidth", "percent-u"} {
		items = append(items, fmt.Sprintf(`{"Id":%d,"Name":%q}`, i, rvForms()[k]))
		i++
	}
	items = append(items, `{"Id":99,"Name":"Beta Industries"}`)
	body := `{"Data":[` + strings.Join(items, ",") + `],"Meta":{"ResponseCode":200,"TotalCount":6}}`
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, body)
	}))
	t.Cleanup(srv.Close)
	return trustTestServer(t, srv)
}

// rvReceiver records every body POSTed to it (a webhook or feedback endpoint).
type rvReceiver struct {
	mu     sync.Mutex
	bodies [][]byte
	url    string
}

func newRVReceiver(t *testing.T) *rvReceiver {
	t.Helper()
	rc := &rvReceiver{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		rc.mu.Lock()
		rc.bodies = append(rc.bodies, b)
		rc.mu.Unlock()
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(srv.Close)
	rc.url = srv.URL + "/ingest"
	return rc
}

func (rc *rvReceiver) all() []byte {
	rc.mu.Lock()
	defer rc.mu.Unlock()
	return bytes.Join(rc.bodies, []byte("\n"))
}

func rvEnv(t *testing.T, host string) *bbEnv {
	t.Helper()
	e := newEnv(t, host, false)
	e.env = append(e.env, "KASEYA_VSA_HOME="+e.home,
		"KASEYA_VSA_TOKEN_ID="+rvTokenID, "KASEYA_VSA_TOKEN_SECRET="+rvTokenSecret)
	return e
}

// rvScanFiles checks every file under home; it returns how many it read.
func rvScanFiles(t *testing.T, home string) int {
	t.Helper()
	n := 0
	_ = filepath.WalkDir(home, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		data, rerr := os.ReadFile(path)
		if rerr != nil {
			return nil
		}
		n++
		rel, _ := filepath.Rel(home, path)
		rvAssertNone(t, "file "+rel, data)
		return nil
	})
	return n
}

// P1-A: no store column keeps a credential, the ID included.
func TestR1ReviewHeldIDNeverStored(t *testing.T) {
	e := rvEnv(t, rvServer(t))
	out := e.run(t, "sync", "--resources", "organizations")
	rvAssertNone(t, "sync output", []byte(out))
	if rvScanFiles(t, e.home) == 0 {
		t.Fatal("sync wrote no files")
	}
	var stored bool
	_ = filepath.WalkDir(e.home, func(path string, d fs.DirEntry, err error) error {
		if err == nil && !d.IsDir() && strings.Contains(d.Name(), ".db") {
			if b, rerr := os.ReadFile(path); rerr == nil && bytes.Contains(b, []byte("Beta Industries")) {
				stored = true
			}
		}
		return nil
	})
	if !stored {
		t.Fatalf("the ordinary record was not stored, so the scan proves nothing; sync said:\n%s", out)
	}
}

// P1-B: every encoding the client masker knows is withheld from CLI output,
// MCP results and --deliver webhook.
func TestR1ReviewClientMaskerEncodingsNeverPrinted(t *testing.T) {
	e := rvEnv(t, rvServer(t))
	hook := newRVReceiver(t)
	out := e.run(t, "organizations", "list", "--data-source", "live", "--json")
	rvAssertNone(t, "organizations list --json", []byte(out))
	if !strings.Contains(out, "Beta Industries") {
		t.Fatalf("the ordinary record is missing, so the check proves nothing:\n%s", out)
	}
	rvAssertNone(t, "organizations list (table)", []byte(e.run(t, "organizations", "list", "--data-source", "live")))
	dout := e.run(t, "organizations", "list", "--data-source", "live", "--json", "--deliver", "webhook:"+hook.url)
	rvAssertNone(t, "--deliver output", []byte(dout))
	got := hook.all()
	if !bytes.Contains(got, []byte("Beta Industries")) {
		t.Fatalf("webhook received no record (output %s)", dout)
	}
	rvAssertNone(t, "webhook body", got)
	mcp := e.mcpCalls(t, []map[string]any{
		{"name": "kaseya-vsa_read", "arguments": map[string]any{"endpoint_id": "organizations.list"}},
		{"name": "organizations_list", "arguments": map[string]any{"data-source": "live"}},
	})
	if !strings.Contains(mcp, "Beta Industries") {
		t.Fatalf("MCP results lost the ordinary record:\n%.2000s", mcp)
	}
	rvAssertNone(t, "MCP session", []byte(mcp))
}

// P1-C: the learn journal, feedback.jsonl and the feedback endpoint never
// receive a credential.
func TestR1ReviewLedgersNeverKeepACredential(t *testing.T) {
	e := rvEnv(t, rvServer(t))
	fb := newRVReceiver(t)
	e.env = append(e.env, "KASEYA_VSA_FEEDBACK_ENDPOINT="+fb.url)
	outs := e.run(t, "--"+rvTokenSecret)
	outs += e.run(t, "organizations", "list", "--"+rvRot13(rvTokenSecret))
	outs += e.run(t, "feedback", rvTokenSecret)
	outs += e.run(t, "feedback", "upper "+strings.ToUpper(rvTokenSecret), "--send")
	outs += e.run(t, "feedback", "plain words survive", "--send")
	rvAssertNone(t, "command output", []byte(outs))
	rvScanFiles(t, e.home)
	got := fb.all()
	if !bytes.Contains(got, []byte("plain words survive")) {
		t.Fatalf("the feedback endpoint received nothing, so the check proves nothing; output:\n%s", outs)
	}
	rvAssertNone(t, "feedback endpoint body", got)
	var journal, ledger bool
	_ = filepath.WalkDir(e.home, func(path string, d fs.DirEntry, err error) error {
		if err == nil && !d.IsDir() {
			journal = journal || strings.HasPrefix(d.Name(), "journal-")
			ledger = ledger || d.Name() == "feedback.jsonl"
		}
		return nil
	})
	if !journal || !ledger {
		t.Fatalf("journal written: %v, feedback.jsonl written: %v; the scan proves nothing", journal, ledger)
	}
}

// Review round 2 (Codex, 2026-10-28) [P1], the repro as reported: the MCP
// HTTP token is the ASCII85 of the token secret, and the token's ROT13 must
// reach neither feedback.jsonl nor the feedback endpoint.
func TestR2ReviewACredentialThatEncodesAnotherIsStillHeld(t *testing.T) {
	secret := strings.Repeat("0", 32)
	token := strings.Repeat("0JG17", 8)
	echo := rvRot13(token)
	e := newEnv(t, rvServer(t), false)
	fb := newRVReceiver(t)
	e.env = append(e.env, "KASEYA_VSA_HOME="+e.home, "KASEYA_VSA_TOKEN_SECRET="+secret,
		"KASEYA_VSA_MCP_HTTP_TOKEN="+token, "KASEYA_VSA_FEEDBACK_ENDPOINT="+fb.url)
	out := e.run(t, "feedback", echo)
	out += e.run(t, "feedback", echo, "--send")
	out += e.run(t, "feedback", "plain words survive", "--send")
	none := func(where string, data []byte) {
		t.Helper()
		low := bytes.ToLower(data)
		for _, f := range []string{echo, token, secret} {
			if bytes.Contains(low, []byte(strings.ToLower(f))) {
				t.Errorf("%s holds a credential as %q", where, f)
			}
		}
	}
	none("command output", []byte(out))
	ledger := false
	_ = filepath.WalkDir(e.home, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		ledger = ledger || d.Name() == "feedback.jsonl"
		if data, rerr := os.ReadFile(path); rerr == nil {
			rel, _ := filepath.Rel(e.home, path)
			none("file "+rel, data)
		}
		return nil
	})
	got := fb.all()
	if !ledger || !bytes.Contains(got, []byte("plain words survive")) {
		t.Fatalf("feedback.jsonl written: %v, endpoint received: %q; the check proves nothing", ledger, got)
	}
	none("feedback endpoint body", got)
}
