// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"

	"eset-protect-pp-cli/internal/cliutil/testenv"
)

func TestC1EscapeWriterKeepsJSONValidAndTerminalSafe(t *testing.T) {
	enc, _ := json.Marshal(map[string]string{"name": "evil\u009b2J\u009d52;c;aGk=\u009cX", "ok": "Zürich  "})
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
	if back["name"] != "evil\u009b2J\u009d52;c;aGk=\u009cX" || back["ok"] != "Zürich  " {
		t.Fatalf("round trip changed the data: %q", back)
	}
	if isTerminal(&c1EscapeWriter{w: &out}) {
		t.Fatal("a wrapped buffer must not read as a terminal")
	}
}

// captureExecute runs the real Execute (the path main.go takes) with
// os.Stdout swapped for a pipe, so the writer Execute installs is the one
// under test, not a test-supplied buffer.
func captureExecute(t *testing.T, args ...string) (string, error) {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	oldOut, oldArgs := os.Stdout, os.Args
	os.Stdout = w
	os.Args = append([]string{"eset-protect-cli"}, args...)
	done := make(chan []byte)
	go func() {
		b, _ := io.ReadAll(r)
		done <- b
	}()
	runErr := Execute()
	_ = w.Close()
	os.Stdout, os.Args = oldOut, oldArgs
	return string(<-done), runErr
}

// A device name carrying UTF-8 C1 controls (CSI U+009B, OSC U+009D, ST
// U+009C) must reach stdout escaped, in JSON output and through --deliver.
func TestExecuteEscapesC1ControlsOnStdout(t *testing.T) {
	testenv.Isolate(t)
	respondClearHarness(t)
	const uuid = "00000000-0000-0000-0000-000000000001"
	respondFakeESET(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/devices/"+uuid {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		// Raw UTF-8 C1 bytes in the body (an interpreted Go string), the way
		// a tenant-controlled name arrives; a pre-escaped \u009b would pass
		// through verbatim and prove nothing.
		_, _ = io.WriteString(w, "{\"device\":{\"uuid\":\""+uuid+"\",\"displayName\":\"PC\u009b2J\u009d52;c;aGk=\u009c01\"}}")
	})
	for name, extra := range map[string][]string{
		"json":    {"--json"},
		"deliver": {"--json", "--deliver", "file:" + t.TempDir() + "/out.json"},
	} {
		out, err := captureExecute(t, append([]string{"devices", "get", uuid}, extra...)...)
		if err != nil {
			t.Fatalf("%s: devices get: %v\n%s", name, err, out)
		}
		if strings.ContainsAny(out, "\u009b\u009c\u009d") {
			t.Fatalf("%s: raw C1 reached stdout: %q", name, out)
		}
		if !strings.Contains(out, `PC\u009b2J`) {
			t.Fatalf("%s: device name lost or not escaped: %q", name, out)
		}
	}
}
