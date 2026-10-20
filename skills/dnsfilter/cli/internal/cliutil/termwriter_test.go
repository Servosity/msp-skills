// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cliutil

import (
	"bytes"
	"strings"
	"testing"
)

func rawControl(s string) (int, bool) {
	for i := 0; i < len(s); i++ {
		b := s[i]
		if (b < 0x20 && b != '\t' && b != '\n' && b != '\r') || b == 0x7f || b == 0x1b {
			return i, true
		}
	}
	for i, r := range s {
		if r >= 0x80 && r <= 0x9f {
			return i, true
		}
	}
	return 0, false
}

// API error text with OSC 52, CSI, BEL, a bare CR, NUL, DEL and C1 reaches
// stderr with every control escaped; tab, newline and CRLF survive.
func TestTerminalWriterEscapesControls(t *testing.T) {
	var out bytes.Buffer
	w := NewTerminalWriter(&out)
	in := "Error: GET /v1/organizations returned HTTP 422: bad \x1b]52;c;cHduZWQ=\x07 thing \u009b31m \x1b[2J end\rfake\x00\x7f\ttab\r\nnext\n"
	if _, err := w.Write([]byte(in)); err != nil {
		t.Fatal(err)
	}
	got := out.String()
	if i, bad := rawControl(strings.ReplaceAll(got, "\r\n", "\n")); bad {
		t.Fatalf("raw control at %d survived: %q", i, got)
	}
	for _, want := range []string{`bad \u001b]52;c;cHduZWQ=\u0007 thing \u009b31m \u001b[2J end\u000dfake\u0000\u007f`, "\ttab\r\nnext\n"} {
		if !strings.Contains(got, want) {
			t.Fatalf("output %q lacks %q", got, want)
		}
	}
}

// The CLI's own colors pass through; any other ESC sequence does not.
func TestTerminalWriterKeepsOnlyTheCLIsOwnSGR(t *testing.T) {
	var out bytes.Buffer
	w := NewTerminalWriter(&out)
	in := "\x1b[32mOK\x1b[0m \x1b[1mbold\x1b[0m \x1b[31mFAIL\x1b[0m \x1b[33mWARN\x1b[0m tenant:\x1b[8mhidden\x1b[38;5;196m\x1b]0;title\x07"
	if _, err := w.Write([]byte(in)); err != nil {
		t.Fatal(err)
	}
	want := "\x1b[32mOK\x1b[0m \x1b[1mbold\x1b[0m \x1b[31mFAIL\x1b[0m \x1b[33mWARN\x1b[0m tenant:\\u001b[8mhidden\\u001b[38;5;196m\\u001b]0;title\\u0007"
	if out.String() != want {
		t.Fatalf("got  %q\nwant %q", out.String(), want)
	}
}

// A UTF-8 C1 split across two writes, and a lone 8-bit C1 byte from a
// non-UTF-8 body, are both escaped; ordinary UTF-8 is untouched.
func TestTerminalWriterSplitAndLoneC1(t *testing.T) {
	var out bytes.Buffer
	w := NewTerminalWriter(&out)
	for _, chunk := range [][]byte{[]byte("Zürich \xc2"), []byte("\x9d52;c;QQ==\x9c x\x9by")} {
		if _, err := w.Write(chunk); err != nil {
			t.Fatal(err)
		}
	}
	if got, want := out.String(), `Zürich \u009d52;c;QQ==\u009c x\u009by`; got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}

// Bidi embedding, override and isolate controls are escaped on stderr;
// ScrubTerminal leaves them (it shapes data, the writers own the terminal);
// other RTL text is untouched.
func TestBidiControlsEscaped(t *testing.T) {
	in := "allow \u202egnp.exe\u202c via \u2066policy\u2069 שלום"
	var out bytes.Buffer
	if _, err := NewTerminalWriter(&out).Write([]byte(in)); err != nil {
		t.Fatal(err)
	}
	want := `allow \u202egnp.exe\u202c via \u2066policy\u2069 ` + "שלום"
	if out.String() != want {
		t.Fatalf("stderr writer: got %q want %q", out.String(), want)
	}
	if got := ScrubTerminal(in + "\x1b[2J\t"); got != in+"[2J " {
		t.Fatalf("ScrubTerminal must drop controls but keep bidi data for the terminal writer: got %q", got)
	}
	for r := rune(0x2000); r < 0x2100; r++ {
		if IsBidiControl(r) != ((r >= 0x202a && r <= 0x202e) || (r >= 0x2066 && r <= 0x2069)) {
			t.Fatalf("IsBidiControl(%U) wrong", r)
		}
	}
}
