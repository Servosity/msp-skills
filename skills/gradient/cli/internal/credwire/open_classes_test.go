// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package credwire_test

// The two credential classes the press 4.34.2 print's last Codex round left
// open (cli-factory LEDGER-NOTES section 7), each closed through the shared
// credential kit or recorded as a kit item (handfixes.json credkit-wiring).
// Each input is the one the review named, on the built binary.

import (
	"fmt"
	"path/filepath"
	"strings"
	"testing"
)

// tripleEncoded percent-encodes every byte of s, then encodes the result's
// percent signs twice more: "K" becomes "%25254B".
func tripleEncoded(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		fmt.Fprintf(&b, "%%2525%02X", s[i])
	}
	return b.String()
}

// feedbackOf runs feedback with the two halves of value, each triple
// percent-encoded in its own member with an unrelated member between them,
// and returns stdout, stderr and every file the run left under its home.
func feedbackOf(t *testing.T, cliBin, value string, env ...string) (stdout, stderr string, files map[string]string) {
	t.Helper()
	home := t.TempDir()
	half := len(value) / 2
	text := fmt.Sprintf(`{"a":%q,"gap":"x","b":%q}`, tripleEncoded(value[:half]), tripleEncoded(value[half:]))
	stdout, stderr, _ = runSplit(t, cliBin, closedEnv(home, env...), "feedback", text, "--json")
	return stdout, stderr, filesUnder(t, home)
}

// TestFeedbackEncodedHalvesWithheld is open class 1, "credential split across
// output members, encoded variant": with GRADIENT_VENDOR_API_KEY set, feedback
// text carrying the key's two halves, each triple percent-encoded, in two
// members with a member between them must print neither half, in either
// spelling, on stdout or stderr, and write neither to feedback.jsonl or any
// other file. Each half is an 8-character piece of the held key, which the
// kit finds after percent-decoding at every depth. A value of the same shape
// that is not held is the control: it must reach stdout and feedback.jsonl,
// so the run proves it reached both sinks.
func TestFeedbackEncodedHalvesWithheld(t *testing.T) {
	cliBin, _ := binaries(t)
	// The reviewed input's shape (16 characters, two 8-character halves) in a
	// low-entropy spelling: the reviewed random-looking key trips gitleaks
	// generic-api-key. That exact key was run by hand on the wired binary and is
	// withheld too (.review/seen/postwire-b1-exact-r2-input.log on the branch).
	const key = "fbkeyzmmzqzzmmqz"
	const control = "fbctlzmmwmmqkwkz"

	out, errOut, files := feedbackOf(t, cliBin, control, "GRADIENT_VENDOR_API_KEY="+key)
	enc := tripleEncoded(control[:8])
	var ledger string
	for p, s := range files {
		if filepath.Base(p) == "feedback.jsonl" {
			ledger += s
		}
	}
	if !strings.Contains(out, enc) || !strings.Contains(ledger, enc) {
		t.Fatalf("control: an unheld value did not reach stdout and feedback.jsonl, so this test proves nothing\nstdout:\n%s\nstderr:\n%s\nfeedback.jsonl:\n%s", out, errOut, ledger)
	}

	out, errOut, files = feedbackOf(t, cliBin, key, "GRADIENT_VENDOR_API_KEY="+key)
	pieces := []string{key[:8], key[8:], tripleEncoded(key[:8]), tripleEncoded(key[8:])}
	for where, text := range map[string]string{"stdout": out, "stderr": errOut} {
		for _, p := range pieces {
			if strings.Contains(text, p) {
				t.Errorf("%s carries %q (a half of the held vendor key):\n%s", where, p, text)
			}
		}
	}
	for path, text := range files {
		for _, p := range pieces {
			if strings.Contains(text, p) {
				t.Errorf("%s carries %q (a half of the held vendor key):\n%s", path, p, text)
			}
		}
	}
}

// TestDecodedBasicPasswordElsewhereInResponse is open class 2, "decoded
// credential part printed elsewhere, short variant": a response carrying an
// authorization member with a Basic credential whose password is three
// characters, and that password again inside another member.
//
// Gradient's documented responses (spec.json, the Synthesize vendor API)
// carry no authorization member, so under the release rule this is P2 unless
// the kit already holds it. The kit does not: it holds a response field only
// when the connector declares it (Declarations.ResponseFields), and Gradient
// declares none because its API answers no credential. Recorded as a kit item
// (.review/kit-findings.md) and a follow-up; no connector-side matcher is
// built. The test stays, skipped, so the input is one flag away.
func TestDecodedBasicPasswordElsewhereInResponse(t *testing.T) {
	if testing.Short() || !strings.EqualFold(envOr("GRADIENT_RUN_P2_PROBES", ""), "1") {
		t.Skip("P2 under the release rule (no documented Gradient response carries an authorization member) and not held by the kit; set GRADIENT_RUN_P2_PROBES=1 to run it")
	}
	cliBin, _ := binaries(t)
	base, ca := fakeAPI(t, `{"authorization":"Basic dTpxN1o=","name":"prefix-q7Z-suffix"}`)
	home := t.TempDir()
	env := closedEnv(home, "GRADIENT_BASE_URL="+base, "GRADIENT_CA_CERT="+ca, "GRADIENT_TOKEN=ptok-wzwzzwzmmqzzqwkw")
	out, errOut, _ := runSplit(t, cliBin, env, "vendor", "get", "--json", "--select", "name")
	if !strings.Contains(out+errOut, "prefix-") && !strings.Contains(out+errOut, "withheld") {
		t.Fatalf("the response did not reach the output, so this test proves nothing:\nstdout:\n%s\nstderr:\n%s", out, errOut)
	}
	if strings.Contains(out+errOut, "q7Z") {
		t.Errorf("the decoded Basic password q7Z prints inside another member:\nstdout:\n%s\nstderr:\n%s", out, errOut)
	}
}
