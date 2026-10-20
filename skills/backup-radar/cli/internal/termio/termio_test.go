// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package termio

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"testing"
)

func TestC1EscapeWriterKeepsJSONValidAndTerminalSafe(t *testing.T) {
	enc, _ := json.Marshal(map[string]string{"name": "evil\u009b2J\u009d52;c;aGk=\u009cX", "ok": "Zürich  "})
	var out bytes.Buffer
	w := New(&out)
	// Split inside a C1 sequence: the 0xC2 lead byte ends the first Write.
	cut := bytes.IndexByte(enc, 0xC2) + 1
	if _, err := w.Write(enc[:cut]); err != nil {
		t.Fatal(err)
	}
	if _, err := w.Write(enc[cut:]); err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(out.Bytes(), []byte{0xC2, 0x9b}) || bytes.Contains(out.Bytes(), []byte{0xC2, 0x9d}) || bytes.Contains(out.Bytes(), []byte{0xC2, 0x9c}) {
		t.Fatalf("raw C1 survived: %q", out.Bytes())
	}
	var back map[string]string
	if err := json.Unmarshal(out.Bytes(), &back); err != nil {
		t.Fatalf("escaped output is not valid JSON: %v (%q)", err, out.Bytes())
	}
	if back["name"] != "evil\u009b2J\u009d52;c;aGk=\u009cX" || back["ok"] != "Zürich  " {
		t.Fatalf("round trip changed the data: %q", back)
	}
	// A 3- and 4-byte rune split across writes survives intact.
	out.Reset()
	w = New(&out)
	in := []byte("a€b😀c")
	for i := range in {
		if _, err := w.Write(in[i : i+1]); err != nil {
			t.Fatal(err)
		}
	}
	if out.String() != string(in) {
		t.Fatalf("byte-at-a-time writes changed %q to %q", in, out.String())
	}
}

func TestC1EscapeWriterNeutralisesC0Controls(t *testing.T) {
	// An OSC 52 clipboard write (ESC ] 52 ; c ; ... BEL) inside a tenant
	// string reaching stderr through an error message, plus DEL and a NUL.
	in := []byte("Error: \"Acme\" matches 2 clients (Acme\x1b]52;c;VEVTVA==\x07 (client id 1), Acme\x00\x7f (client id 2))\n\tok\r\n")
	var out bytes.Buffer
	w := New(&out)
	if _, err := w.Write(in); err != nil {
		t.Fatal(err)
	}
	got := out.String()
	for _, raw := range []byte{0x1b, 0x07, 0x00, 0x7f} {
		if bytes.IndexByte(out.Bytes(), raw) >= 0 {
			t.Fatalf("raw control byte %#x survived: %q", raw, got)
		}
	}
	for _, want := range []string{`\u001b]52;c;VEVTVA==\u0007`, `\u0000\u007f`, "\n\tok\r\n", "(client id 1)"} {
		if !bytes.Contains(out.Bytes(), []byte(want)) {
			t.Fatalf("expected %q in %q", want, got)
		}
	}
	// JSON output is untouched: encoding/json already escaped the control.
	enc, _ := json.Marshal(map[string]string{"name": "x\x1by"})
	out.Reset()
	if _, err := New(&out).Write(enc); err != nil {
		t.Fatal(err)
	}
	if out.String() != string(enc) {
		t.Fatalf("JSON changed: %q -> %q", enc, out.String())
	}
}

func TestC1EscapeWriterKeepsOnlyTheCLIsOwnSGR(t *testing.T) {
	cases := map[string]string{
		"\x1b[32mOK\x1b[0m \x1b[1;31mFAIL\x1b[0m \x1b[1mhdr\x1b[0m \x1b[33mwarn\x1b[0m \x1b[31mbad\x1b[0m": "\x1b[32mOK\x1b[0m \x1b[1;31mFAIL\x1b[0m \x1b[1mhdr\x1b[0m \x1b[33mwarn\x1b[0m \x1b[31mbad\x1b[0m", // the CLI's own colors survive
		"\x1b]52;c;VEVTVA==\x07":                `\u001b]52;c;VEVTVA==\u0007`,              // OSC 52 clipboard write
		"\x1b[2J\x1b[H":                         `\u001b[2J\u001b[H`,                       // non-SGR CSI
		"\x1b]0;title\x1b\\":                    `\u001b]0;title\u001b\`,                   // OSC title with ST
		"\x1b[31":                               `\u001b[31`,                               // unterminated
		"\x1b[?25l":                             `\u001b[?25l`,                             // private-mode CSI (cursor hide)
		"\x1b[8mHIDDEN\x1b[0m":                  `\u001b[8mHIDDEN` + "\x1b[0m",             // conceal is not the CLI's; reset is
		"\x1b[38;5;196mred":                     `\u001b[38;5;196mred`,                     // 256-color SGR
		"\x1b[0;1;2;3;4;5;6;7;8;9;10;11;12;13m": `\u001b[0;1;2;3;4;5;6;7;8;9;10;11;12;13m`, // over the parameter cap
		"REAL\rfake":                            `REAL\u000dfake`,                          // a bare CR overwrites the line
		"line\r\nnext":                          "line\r\nnext",                            // CRLF is a line ending
		"x\x9d52;c;Zm9v\x9cy":                   `x\u009d52;c;Zm9v\u009cy`,                 // lone 8-bit C1 bytes (not UTF-8)
	}
	for in, want := range cases {
		var out bytes.Buffer
		if _, err := New(&out).Write([]byte(in)); err != nil {
			t.Fatal(err)
		}
		if out.String() != want {
			t.Fatalf("%q -> %q, want %q", in, out.String(), want)
		}
	}
	// A CR or a possible UTF-8 lead byte that ends one Write is decided by the next one.
	for _, tc := range []struct{ a, b, want string }{{"x\r", "\ny", "x\r\ny"}, {"x\r", "y", `x\u000dy`}, {"caf\xe9", "!", "caf\xe9!"}} { // a lone Latin-1 byte is not a control
		var out bytes.Buffer
		w := New(&out)
		w.Write([]byte(tc.a))
		w.Write([]byte(tc.b))
		if out.String() != tc.want {
			t.Fatalf("%q+%q -> %q, want %q", tc.a, tc.b, out.String(), tc.want)
		}
	}
}

func TestEscapeStringEscapesEveryControl(t *testing.T) {
	in := "Evil\x1b]52;c;QQ==\x07 \u009d0;PWNED\u009c \x1b[2J \x1b[8m Corp\tok\n"
	want := `Evil\u001b]52;c;QQ==\u0007 \u009d0;PWNED\u009c \u001b[2J \u001b[8m Corp` + "\tok\n"
	if got := EscapeString(in); got != want {
		t.Fatalf("EscapeString: %q, want %q", got, want)
	}
	if got := EscapeString("plain Zürich"); got != "plain Zürich" {
		t.Fatalf("EscapeString changed plain text: %q", got)
	}
	// A bare CR (a provider error message trying to overwrite the line) is
	// escaped; CRLF is a line ending and passes.
	if got := EscapeString("REAL\rfake\r\nnext\r"); got != `REAL\u000dfake`+"\r\nnext"+`\u000d` {
		t.Fatalf("EscapeString CR handling: %q", got)
	}
}

func TestProgressKeepsOnlyItsOwnCR(t *testing.T) {
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	old := os.Stderr
	os.Stderr = w
	Progress("  %s: %d synced", "dev\rFAKE\x1b]52;c;QQ==\x07", 5)
	os.Stderr = old
	w.Close()
	got, _ := io.ReadAll(r)
	if !bytes.HasPrefix(got, []byte("\r  dev")) || bytes.Count(got, []byte("\r")) != 1 || bytes.ContainsAny(got, "\x1b\x07") || !bytes.Contains(got, []byte(`\u000d`)) {
		t.Fatalf("progress line = %q, want one leading CR and every control in the text escaped", got)
	}
}
