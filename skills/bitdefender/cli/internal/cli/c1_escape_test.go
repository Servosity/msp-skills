// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"bytes"
	"encoding/json"
	"testing"
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
