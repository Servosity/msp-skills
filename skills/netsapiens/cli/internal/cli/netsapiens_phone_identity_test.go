// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

// Class test (identifier-disambiguation): a phone number is compared as the
// E.164 number it is (Phonenumber.phonenumber is documented E.164 with its
// country code). An explicit country code is kept; an input that could be
// more than one stored number fails closed with the candidates, never picks
// one. The Codex round-1 input is `trace +6495550142` against a mirror holding
// 16495550142 (+1 649, another tenant): it matched.

package cli

import (
	"path/filepath"
	"strings"
	"testing"

	"netsapiens-pp-cli/internal/cliutil/testenv"
)

func TestClassPhoneNumbersKeepTheirCountryCode(t *testing.T) {
	const nanp = `{"phonenumber":"16495550142","domain":"a.example","dial-rule-application":"hangup"}`
	const nz = `{"phonenumber":"6495550142","domain":"b.example","dial-rule-application":"hangup"}`
	seed := func(t *testing.T, numbers ...string) string {
		path := filepath.Join(t.TempDir(), "data.db")
		seedCovered(t, path, "domains", `{"domain":"a.example"}`, `{"domain":"b.example"}`)
		seedCovered(t, path, "phonenumbers", numbers...)
		seedCovered(t, path, "smsnumbers")
		seedCovered(t, path, "users")
		return path
	}
	trace := func(t *testing.T, path, q string, format ...string) map[string]any {
		if len(format) == 0 {
			format = []string{"--json"}
		}
		rep := decodeObject(t, mustRunFleet(t, append([]string{"trace", q, "--db", path}, format...)...))
		if r, ok := rep["results"].(map[string]any); ok { // the --agent envelope
			return r
		}
		return rep
	}
	t.Run("a tel: URI is a number", func(t *testing.T) {
		if kinds := traceKinds("tel:+6495550142"); kinds[0] != "number" {
			t.Fatalf("kinds %v", kinds)
		}
	})
	t.Run("an explicit country code never matches another country's number", func(t *testing.T) {
		testenv.Isolate(t)
		rep := trace(t, seed(t, nanp), "+6495550142")
		if n := len(rep["matches"].([]any)); n != 0 {
			t.Fatalf("+64 9 555 0142 matched +1 649 555 0142 (another tenant): %v", rep)
		}
	})
	t.Run("an international marker in any spelling never yields a NANP candidate", func(t *testing.T) {
		testenv.Isolate(t)
		// Codex round-2 input: "(+64) 9 555 0142" fell through to the +1 649 reading.
		only := seed(t, nanp)
		both := seed(t, nanp, nz)
		for _, q := range []string{"(+64) 9 555 0142", "( +64 ) 9-555-0142", "+64 9 555 0142", "0064 9 555 0142", "01164 9 555 0142"} {
			if n := len(trace(t, only, q)["matches"].([]any)); n != 0 {
				t.Fatalf("%s is +64 9 555 0142; it matched +1 649 555 0142 (another tenant)", q)
			}
			rep := trace(t, both, q)
			if m := rep["matches"].([]any); len(m) != 1 || m[0].(map[string]any)["domain"] != "b.example" {
				t.Fatalf("%s must match +64 9 555 0142 (b.example) alone: %v", q, rep)
			}
		}
	})
	t.Run("an input that could be two stored numbers fails closed with both", func(t *testing.T) {
		testenv.Isolate(t)
		path := seed(t, nanp, nz)
		for _, format := range []string{"--json", "--agent"} {
			rep := trace(t, path, "6495550142", format)
			if m, _ := rep["matches"].([]any); len(m) != 0 || rep["complete"] == true {
				t.Fatalf("%s: 6495550142 is +1 649 555 0142 or +64 9 555 0142; one was picked: %v", format, rep)
			}
			c := stringsOf(rep["ambiguous_candidates"])
			if !strings.Contains(c, "a.example") || !strings.Contains(c, "b.example") {
				t.Fatalf("%s: the candidates must be listed: %v", format, rep)
			}
		}
	})
	t.Run("one number in any documented spelling still matches", func(t *testing.T) {
		testenv.Isolate(t)
		path := seed(t, nanp, nz)
		for q, dom := range map[string]string{"+1 (649) 555-0142": "a.example", "16495550142": "a.example", "+64 9 555 0142": "b.example", "0064 9 555 0142": "b.example"} {
			rep := trace(t, path, q)
			if m := rep["matches"].([]any); len(m) != 1 || m[0].(map[string]any)["domain"] != dom {
				t.Fatalf("%s must match %s alone: %v", q, dom, rep)
			}
		}
	})
}
