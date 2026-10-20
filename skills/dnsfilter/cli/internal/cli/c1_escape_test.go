// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"dnsfilter-pp-cli/internal/cliutil/testenv"
)

func TestC1EscapeWriterKeepsJSONValidAndTerminalSafe(t *testing.T) {
	enc, _ := json.Marshal(map[string]string{"name": "evil\u009b2J\u009d52;c;aGk=\u009cX", "ok": "Zürich  "})
	var out bytes.Buffer
	w := &c1EscapeWriter{w: &out}
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
	if back["name"] != "evil\u009b2J\u009d52;c;aGk=\u009cX" || back["ok"] != "Zürich  " {
		t.Fatalf("round trip changed the data: %q", back)
	}
	if isTerminal(&c1EscapeWriter{w: &out}) {
		t.Fatal("a wrapped buffer must not read as a terminal")
	}
}

// Execute wires the escape around stdout: a tenant name carrying C1 CSI/OSC
// in a real command's JSON output reaches the terminal escaped.
func TestExecuteEscapesC1OnStdout(t *testing.T) {
	testenv.Isolate(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, "{\"data\":{\"id\":\"1\",\"attributes\":{\"name\":\"Acme\u009b2J\u009d52;c;aGk=\u009c\"}}}")
	}))
	t.Cleanup(srv.Close)
	t.Setenv("DNSFILTER_BASE_URL", srv.URL)
	t.Setenv("DNSFILTER_API_KEY", "fixture-key")
	fakeStdoutTerminal(t, true)
	stdout, stderr, err := executeArgs(t, "organizations", "show", "1", "--json")
	if err != nil {
		t.Fatalf("organizations show: %v\n%s%s", err, stdout, stderr)
	}
	if strings.ContainsAny(stdout, "\u009b\u009c\u009d") {
		t.Fatalf("raw C1 reached stdout: %q", stdout)
	}
	if !strings.Contains(stdout, `Acme\u009b2J`) {
		t.Fatalf("tenant name missing or not escaped: %q", stdout)
	}
}

// isTerminal sees through the wrapper, so wrapping stdout does not turn a
// terminal's human tables into JSON. The null device is a character device,
// which is what isTerminal tests for.
func TestIsTerminalUnwrapsC1EscapeWriter(t *testing.T) {
	f, err := os.OpenFile(os.DevNull, os.O_WRONLY, 0)
	if err != nil {
		t.Skip(err)
	}
	defer f.Close()
	if !isTerminal(f) {
		t.Skip("the null device is not a character device here")
	}
	if !isTerminal(&c1EscapeWriter{w: f}) {
		t.Fatal("isTerminal does not unwrap c1EscapeWriter")
	}
}

// Lone 8-bit C1 bytes (from a raw, non-UTF-8 API body) and bidi controls are
// escaped on stdout; valid multibyte sequences, split across writes or not,
// and a CSV cell's quoted CR pass through unchanged.
func TestC1EscapeWriterLoneC1BidiAndMultibyte(t *testing.T) {
	var out bytes.Buffer
	w := &c1EscapeWriter{w: &out}
	chunks := [][]byte{
		[]byte("{\"name\":\"Raw\x9b31mRAW9B \xe2"), // lone CSI byte; the euro sign is split
		[]byte("\x82\xac Zürich \U0001F600 中 \u202eevil\u2066x\u2069\"}\n"),
		[]byte("\"a\r\nb\",c\r\n"), // quoted CSV cell with CRLF inside
	}
	for _, ch := range chunks {
		if _, err := w.Write(ch); err != nil {
			t.Fatal(err)
		}
	}
	got := out.String()
	want := `{"name":"Raw\u009b31mRAW9B ` + "€ Zürich \U0001F600 中 " + `\u202eevil\u2066x\u2069"}` + "\n\"a\r\nb\",c\r\n"
	if got != want {
		t.Fatalf("got  %q\nwant %q", got, want)
	}
	enc, _ := json.Marshal(map[string]string{"name": "A\u202eB\u2067C"})
	out.Reset()
	if _, err := (&c1EscapeWriter{w: &out}).Write(enc); err != nil {
		t.Fatal(err)
	}
	var back map[string]string
	if err := json.Unmarshal(out.Bytes(), &back); err != nil || back["name"] != "A\u202eB\u2067C" || bytes.Contains(out.Bytes(), []byte("\u202e")) {
		t.Fatalf("bidi escape must keep JSON valid and lossless with no raw control: %q (%v)", out.Bytes(), err)
	}
}

// The receipt: an organization name carrying a raw 0x9b byte in the API body
// reaches stdout escaped through the real Execute().
func TestExecuteEscapesLoneC1OnStdout(t *testing.T) {
	testenv.Isolate(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte("{\"data\":[{\"id\":\"1\",\"attributes\":{\"name\":\"Raw\x9b31mRAW9B\"}}]}"))
	}))
	t.Cleanup(srv.Close)
	t.Setenv("DNSFILTER_BASE_URL", srv.URL)
	t.Setenv("DNSFILTER_API_KEY", "fixture-key")
	fakeStdoutTerminal(t, true)
	stdout, stderr, err := executeArgs(t, "organizations", "index", "--json")
	if err != nil {
		t.Fatalf("organizations index: %v\n%s%s", err, stdout, stderr)
	}
	if strings.Contains(stdout, "\x9b") {
		t.Fatalf("raw 0x9b reached stdout: %q", stdout)
	}
	if !strings.Contains(stdout, "RAW9B") {
		t.Fatalf("organization missing from output: %q", stdout)
	}
	// A file or pipe is not a terminal: the bytes the API sent are kept.
	fakeStdoutTerminal(t, false)
	stdout, _, err = executeArgs(t, "organizations", "index", "--json", "--no-cache")
	if err != nil || !strings.Contains(stdout, "Raw\x9b31mRAW9B") {
		t.Fatalf("stdout to a file lost the raw byte: err=%v %q", err, stdout)
	}
}

// fakeStdoutTerminal makes the stdout writer treat the process stdout as a
// terminal (or not) for the rest of the test.
func fakeStdoutTerminal(t *testing.T, tty bool) {
	t.Helper()
	old := stdoutIsTerminal
	stdoutIsTerminal = func() bool { return tty }
	t.Cleanup(func() { stdoutIsTerminal = old })
}

// The stdout writer escapes only for a terminal: a raw writer (file, pipe)
// passes every byte, bidi and lone C1 included, unchanged.
func TestStdoutWriterRawWhenNotTerminal(t *testing.T) {
	in := []byte("{\"name\":\"Acme \u202aDental\u202c Raw\x9b31m\"}\n")
	var out bytes.Buffer
	if _, err := (&c1EscapeWriter{w: &out, raw: true}).Write(in); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(out.Bytes(), in) {
		t.Fatalf("raw writer changed the bytes: %q", out.Bytes())
	}
	fakeStdoutTerminal(t, false)
	if !newStdoutWriter().raw {
		t.Fatal("stdout that is not a terminal must be written raw")
	}
	fakeStdoutTerminal(t, true)
	if newStdoutWriter().raw {
		t.Fatal("a terminal stdout must be escaped")
	}
}
