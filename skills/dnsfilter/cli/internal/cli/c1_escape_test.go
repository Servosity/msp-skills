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
