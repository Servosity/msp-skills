// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package credkit

// The members found by probing before the kit6rv round 2, and the classes
// that round raised, each with its input and the class's other members.

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// Class code-list-token-edges (contract 3, Candidates and Character-code
// lists; Reading bounds, runDecoderSpans): a code-list span starts and ends
// at any byte, so a list's first token is its run's digits whatever stands
// before them, and its last token ends where the run of the mode's digits
// ends, whatever follows. A run too large for a code point gives its
// longest prefix that is one to the list it ends, and its longest suffix
// that is one to the list it starts.
func TestKit6CodeListTokenEdges(t *testing.T) {
	holding(t, "secret!!")
	for _, u := range []string{
		"x115,101,99,114,101,116,33,33",            // a letter before the first token
		"ab115,101,99,114,101,116,33,33",           // hex letters before it (decimal reading)
		"115,101,99,114,101,116,33,33x",            // a letter after the last token
		"115,101,99,114,101,116,33,33abc",          // hex letters after it (decimal reading)
		"id=115,101,99,114,101,116,33,33;",         // ordinary framing
		"x0x73,0x65,0x63,0x72,0x65,0x74,0x21,0x21", // hex with prefixes, a letter before
		"0x73,0x65,0x63,0x72,0x65,0x74,0x21,0x21z", // and after
	} {
		if !withheld([]byte(u)) {
			t.Errorf("%q released (a code list of secret!!)", u)
		}
	}
	// The longest-suffix and longest-prefix rule, where it is the only reading.
	holding(t, "\U000F3ECBecret!")
	if !withheld([]byte("q9999115,101,99,114,101,116,33")) { // 9999115 is past 10FFFF; its longest scalar suffix is 999115 (U+F3ECB)
		t.Error("the longest scalar suffix of a first run was not read")
	}
	holding(t, "ecret!!\U00050D6A")
	if !withheld([]byte("101,99,114,101,116,33,33,331114112")) { // 331114112 is past 10FFFF; its longest scalar prefix is 331114 (U+50D6A)
		t.Error("the longest scalar prefix of a last run was not read")
	}
	// Not over-read: a list needs two tokens, and ordinary digits stay clean.
	holding(t, "secret!!")
	for _, u := range []string{"x115", "version 1.2.3, build 4567", "ids: 12, 34, 56"} {
		if withheld([]byte(u)) {
			t.Errorf("%q withheld", u)
		}
	}
}

// Class file-stream-state-reset (contract 2, 3 Streams, 5), the member found
// before round 2: a write that lands before the end of a file (opened
// without O_APPEND or O_TRUNC, or after a seek) completes a credential with
// the bytes already there. It is checked as the file it leaves, and the
// file's stream then continues from the file's own end.
func TestKit6FileWriteBeforeTheEnd(t *testing.T) {
	resetForTest(Declarations{})
	Hold("minted:t", "secret!!")
	dir := t.TempDir()
	read := func(p string) string { b, _ := os.ReadFile(p); return string(b) }

	p := filepath.Join(dir, "over")
	if err := WriteFile(p, []byte("XXXXet!!"), 0o600); err != nil {
		t.Fatal(err)
	}
	w, err := OpenFile(p, os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	w.Write([]byte("secr"))
	w.Close()
	if strings.Contains(read(p), "secret!!") {
		t.Errorf("a write at the start of a file completed the credential: %q", read(p))
	}

	p = filepath.Join(dir, "seek")
	f, err := os.OpenFile(p, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	nw := NewWriter(f, SinkFile)
	nw.Write([]byte("XXXXet!!"))
	f.Seek(0, io.SeekStart)
	nw.Write([]byte("secr"))
	f.Close()
	if strings.Contains(read(p), "secret!!") {
		t.Errorf("a write after a seek completed the credential: %q", read(p))
	}

	// After a write before the end, the stream is the file's own end.
	p = filepath.Join(dir, "resync")
	if err := WriteFile(p, []byte("ab-----secr"), 0o600); err != nil {
		t.Fatal(err)
	}
	w, err = OpenFile(p, os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	w.Write([]byte("cd"))
	w.Close()
	AppendFile(p, []byte("et!!"), 0o600)
	if strings.Contains(read(p), "secret!!") {
		t.Errorf("an append after a write before the end completed the credential: %q", read(p))
	}

	// Clean writes before the end still go through.
	p = filepath.Join(dir, "clean")
	WriteFile(p, []byte("hello world"), 0o600)
	w, _ = OpenFile(p, os.O_WRONLY, 0)
	w.Write([]byte("HELLO"))
	w.Close()
	if read(p) != "HELLO world" {
		t.Errorf("a clean write before the end = %q", read(p))
	}
}

// Class replacement-context-unchecked (contract 4; 3 Streams), kit6rv round
// 2: the notice that replaces a withheld write of a stream is checked with
// what the stream released before it, and dropped when it would complete a
// held value there.
func TestKit6NoticeCheckedWithTheStream(t *testing.T) {
	resetForTest(Declarations{})
	Hold("minted:t", "abcdefg[")
	p := filepath.Join(t.TempDir(), "f")
	if err := WriteFile(p, []byte("abcdefg"), 0o600); err != nil {
		t.Fatal(err)
	}
	AppendFile(p, []byte("abcdefg["), 0o600) // Codex's input
	if b, _ := os.ReadFile(p); strings.Contains(string(b), "abcdefg[") {
		t.Errorf("the notice completed the credential in the file: %q", b)
	}

	// The same on stdout (Codex's held "ne one\n[").
	resetForTest(Declarations{})
	Hold("minted:t", "ne one\n[")
	startedOnce.Do(func() { close(started) })
	inR, inW, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	outR, outW, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	pp := &pump{fd: 1, sink: SinkStdout, in: inR, out: outW, done: make(chan struct{})}
	go pp.run()
	got := make(chan []byte)
	go func() { b, _ := io.ReadAll(outR); got <- b }()
	inW.Write([]byte("clean line one\n"))
	time.Sleep(10 * unitIdle)
	inW.Write([]byte("x ne one\n[ y\n"))
	time.Sleep(10 * unitIdle)
	inW.Close()
	<-pp.done
	outW.Close()
	if out := <-got; strings.Contains(string(out), "ne one\n[") {
		t.Errorf("the notice completed the credential on stdout: %q", out)
	}
}

// Class file-placement-gap-unchecked (contract 2, 3 Streams, 5), kit6rv
// round 2: a write past the end of a file leaves a gap of zero bytes, and
// the file it leaves is what is checked.
func TestKit6FileWritePastTheEnd(t *testing.T) {
	resetForTest(Declarations{})
	Hold("minted:t", "secr\x00et!!")
	p := filepath.Join(t.TempDir(), "f")
	if err := WriteFile(p, []byte("secr"), 0o600); err != nil {
		t.Fatal(err)
	}
	f, err := os.OpenFile(p, os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	f.Seek(5, io.SeekStart)
	NewWriter(f, SinkFile).Write([]byte("et!!")) // Codex's input
	f.Close()
	if b, _ := os.ReadFile(p); strings.Contains(string(b), "secr\x00et!!") {
		t.Errorf("a write past the end completed the credential: %q", b)
	}
}

// Class file-stream-state-reset (contract 3 Streams, 5), kit6rv round 2:
// WriteFile's write and its stream's restart are one step, so an append
// from another goroutine in between is checked after the whole record.
func TestKit6WriteFileAndAppendRace(t *testing.T) {
	resetForTest(Declarations{})
	Hold("minted:t", "secret!!")
	p := filepath.Join(t.TempDir(), "f")
	w, err := Create(p)
	if err != nil {
		t.Fatal(err)
	}
	w.Close()
	done := make(chan struct{})
	writeFileHook = func() {
		writeFileHook = nil
		go func() { AppendFile(p, []byte("et!!"), 0o600); close(done) }()
		time.Sleep(100 * time.Millisecond) // Codex's descheduled WriteFile
	}
	t.Cleanup(func() { writeFileHook = nil })
	WriteFile(p, []byte("secr"), 0o600)
	<-done
	if b, _ := os.ReadFile(p); strings.Contains(string(b), "secret!!") {
		t.Errorf("an append between WriteFile's write and its stream completed the credential: %q", b)
	}
}

// Class charset-start-boundary-loss (contract 3, Candidates and Charsets: a
// reading from every byte offset; Joined values), kit6rv round 2: a reading
// that starts after a prefix holds a JSON document whose string values join
// to a held value, so a document is joined from every place one can start,
// whatever stands before it on the view or the line.
func TestKit6JSONDocumentAfterAPrefix(t *testing.T) {
	holding(t, "secret!!")
	for _, u := range []string{
		`x{"a":"secr","b":"et!!"}`,                         // the reading from offset 1, no decoding before it
		"787b2261223a2273656372222c2262223a2265742121227d", // Codex's input: hex, then that reading
		`data: {"a":"secr","b":"et!!"}`,                    // a server-sent event
		"line one\n2026-11-02 INFO [\"secr\",\"et!!\"]\n",  // a JSON line after a prefix
		`{"bad" {"a":"secr","b":"et!!"}`,                   // after a document that breaks
		`["a["secr","et!!"]`,                               // a start inside an earlier parse's string
		`x{"a":"secr","b":"et!!`,                           // still open at the end
	} {
		if !withheld([]byte(u)) {
			t.Errorf("%q released (joined: secret!!)", u)
		}
	}
	for _, u := range []string{
		`x{"a":"secr"} y {"b":"et!!"}`, // two documents: each is joined on its own
		`{{ .Name }} [INFO] {}`,
	} {
		if withheld([]byte(u)) {
			t.Errorf("%q withheld", u)
		}
	}
}

// Class modal-charset-pruning (contract 3, Charsets, Recursion, Joined
// values), kit6rv round 2: an ISO-2022-JP or HZ reading is joined whole,
// before it is cut to its relevant characters.
func TestKit6ModalReadingJoined(t *testing.T) {
	holding(t, "secret!!")
	for _, u := range []string{
		"{\"\x1b$B$\"\x1b(B\":0,\"a\":\"secr\",\"b\":\"et!!\"}",                    // ISO-2022-JP, one decoding before the join
		"7b221b244224221b2842223a302c2261223a2273656372222c2262223a2265742121227d", // Codex's input: hex, then that
		"[\"\x1b$B$\"\x1b(B\",\"secr\",\"et!!\"]",                                  // the irrelevant character as a value
	} {
		if !withheld([]byte(u)) {
			t.Errorf("%q released (joined: secret!!)", u)
		}
	}
}

// Class code-list-token-edges (contract 3, Candidates and Character-code
// lists; the ninth reading bound decodes only the whole list further, but
// every span inside it is compared), kit6rv round 2: a span that starts or
// ends inside a token reads that token's digits from where it starts or to
// where it ends, and its output is compared.
func TestKit6CodeListSubTokenSpans(t *testing.T) {
	holding(t, "secret!!")
	for _, u := range []string{
		"1115,101,99,114,101,116,33,33",             // Codex's input: the span starts after the first digit
		"115,101,99,114,101,116,33,333",             // Codex's input: the span ends before the last digit
		"1115,101,99,114,101,116,33,333",            // both
		"7,8,1115,101,99,114,101,116,33,333,9,4",    // both, inside a longer list
		"0x173,0x65,0x63,0x72,0x65,0x74,0x21,0x210", // hex, both
	} {
		if !withheld([]byte(u)) {
			t.Errorf("%q released (a span of it decodes to secret!!)", u)
		}
	}
	for _, u := range []string{"2026-11-02T08:21:34.789Z", "ids: 12, 34, 56", "1115,101,99,114,101,116,33", "1115,1101,999,1114"} {
		if withheld([]byte(u)) {
			t.Errorf("%q withheld", u)
		}
	}
}

// Class config-reader-drops-values (contract 1, 2(a), 5), kit6rv round 2:
// a scalar after a tag on a line of its own is read as a value, its quotes
// and escapes decoded and a comment after it left out, however the item
// starts (Codex's input; the kit5 pending-tag path held the source text).
func TestKit6ConfigPendingTagScalar(t *testing.T) {
	for _, tc := range []struct{ content, field, value string }{
		{"auth_header:\n  !!str\n  \"\\u003f\\u0021\\u003f\" # note\n", "auth_header", "?!?"}, // Codex's input
		{"token:\n  !!str\n  '?!?' # note\n", "token", "?!?"},
		{"token:\n  !!str\n  ?!? # note\n", "token", "?!?"},
		{"token:\n  &a\n  \"?\\x21?\" # note\n", "token", "?!?"},
	} {
		gatherString(t, Declarations{ConfigFields: []string{tc.field}}, "c.yaml", tc.content)
		if !isHeld(tc.value) {
			t.Errorf("%q: %q was not held (held: %q)", tc.content, tc.value, heldValues())
		}
	}
}
