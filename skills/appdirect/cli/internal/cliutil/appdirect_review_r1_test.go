// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

package cliutil

import (
	"strings"
	"testing"
)

// Codex r1 (appdirect reprint on 4.34.2) class secret in encoded output: the
// base85 family is matched by alphabet, RFC 1924 (Python's base64.b85encode,
// git binary patches) beside Ascii85 and Z85. The reported input:
// APPDIRECT_CLIENT_SECRET=ABCDEFGH12345678 and feedback 'K|(`BMMg(RF)}kWH8wXm'.
func TestRFC1924Base85OfAHeldValueIsWithheld(t *testing.T) {
	FunnelHold("ABCDEFGH12345678")
	const b85 = "K|(`BMMg(RF)}kWH8wXm" // python3 -c "import base64;print(base64.b85encode(b'ABCDEFGH12345678'))"
	for _, text := range []string{b85, `{"text":"` + b85 + `"}`, "feedback: " + b85 + " thanks"} {
		if !HeldEchoes(text) || strings.Contains(WithholdText(text), b85) {
			t.Errorf("the RFC 1924 base85 of the held secret printed: %q", WithholdText(text))
		}
	}
}

// Codex r1 class short credential in output: a held value shorter than
// shortMinLen was matched only where it stood alone, so
// `APPDIRECT_CLIENT_SECRET=abc appdirect-cli feedback prefixabcsuffix` printed
// it. Every held value is matched anywhere, with its whole-value encodings and
// its per-byte escapes, at any length.
func TestVeryShortHeldValueIsWithheldInsideText(t *testing.T) {
	const short = "Zq9"
	FunnelHold(short)
	for _, text := range []string{
		"prefixZq9suffix",
		`{"text":"prefixZq9suffix"}`,
		"b64 xWnE5x",            // base64 of Zq9
		"hex 5a7139 here",       // hex
		`esc \x5a\x71\x39 done`, // three escaped bytes
		"pct %5A%71%39 done",
	} {
		if !HeldEchoes(text) {
			t.Errorf("%q repeats the 3-character held value but is not withheld", text)
		}
	}
	if got := WithholdText("prefixZq9suffix"); strings.Contains(got, short) {
		t.Errorf("the short credential printed: %q", got)
	}
}

// Codex r1 class credential split across protocol messages (and, on the CLI
// side, across output bursts): each delivery was filtered alone, so two
// deliveries that each held half of a token printed it whole. The second half
// is withheld once the first is out.
func TestHeldValueSplitAcrossDeliveriesIsWithheld(t *testing.T) {
	const tok = "SplitDeliver-8fK2mQ7xR4tW"
	FunnelHold(tok)
	withheld := func() string { return FunnelWithheldText }
	if got := DeliverChecked("tool 'SplitDeliver-8f' not found", withheld); !strings.Contains(got, "SplitDeliver-8f") {
		t.Fatalf("half of the token alone is delivered as is (the test needs it out): %q", got)
	}
	if got := DeliverChecked("tool 'K2mQ7xR4tW' not found", withheld); strings.Contains(got, "K2mQ7xR4tW") {
		t.Fatalf("the delivery that completes the held token printed: %q", got)
	}
	if got := DeliverChecked("an unrelated line", withheld); got != "an unrelated line" {
		t.Fatalf("a later unrelated delivery is withheld: %q", got)
	}
}
