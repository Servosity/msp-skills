// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package credkit

// kit8: one test per round-3 class of the kit7rv review (Codex session
// 01a19a3d, .review/kit7rv-codex-r3.md), each from Codex's own input. The
// file classes assert the file as one unit: whatever the kit let through,
// the file it leaves carries no held value (contract 2, 3 Streams, 5).

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

// fileLeaks reports whether the file at p, read as one unit, carries a
// held value.
func fileLeaks(t *testing.T, p string) (bool, string) {
	t.Helper()
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	return withheld(b), string(b)
}

// Class file-stream-state-reset: Create's truncate and its stream's
// restart are one step for every kit write to the file, so an append from
// another goroutine lands before both or after both.
func TestKit8CreateTruncateAndRestartAreOneStep(t *testing.T) {
	resetForTest(Declarations{})
	Hold("minted:t", "secret!!")
	p := filepath.Join(t.TempDir(), "f")
	if err := WriteFile(p, []byte("xxxx"), 0o600); err != nil {
		t.Fatal(err)
	}
	a, err := OpenFile(p, os.O_WRONLY|os.O_APPEND, 0)
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan struct{})
	openHook = func() { // Codex's Create, descheduled after the truncate
		openHook = nil
		go func() { a.Write([]byte("secr")); close(done) }()
		time.Sleep(100 * time.Millisecond)
	}
	t.Cleanup(func() { openHook = nil })
	c, err := Create(p)
	if err != nil {
		t.Fatal(err)
	}
	<-done
	c.Close()
	a.Write([]byte("et!!"))
	a.Close()
	if bad, got := fileLeaks(t, p); bad {
		t.Errorf("an append between Create's truncate and its restart completed the credential: %q", got)
	}
}

// Class file-placement-race: the kit writes at the offset it checked, so a
// seek between the check and the write does not move the write.
func TestKit8WriteLandsWhereChecked(t *testing.T) {
	resetForTest(Declarations{})
	Hold("minted:t", "secret!!")
	p := filepath.Join(t.TempDir(), "f")
	if err := WriteFile(p, []byte("secrXXXX"), 0o600); err != nil {
		t.Fatal(err)
	}
	f, err := os.OpenFile(p, os.O_RDWR, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	f.Seek(8, io.SeekStart)
	w := NewWriter(f, SinkFile)
	placeHook = func() { placeHook = nil; f.Seek(4, io.SeekStart) } // Codex's seek from another goroutine
	t.Cleanup(func() { placeHook = nil })
	w.Write([]byte("et!!"))
	if bad, got := fileLeaks(t, p); bad {
		t.Errorf("a seek between the check and the write completed the credential: %q", got)
	}
}

// Class file-placement-error-fail-open: when the kit cannot tell what a
// write joins, nothing is written, the notice included.
func TestKit8PlacementErrorWritesNothing(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("renaming an open file: Unix")
	}
	resetForTest(Declarations{})
	Hold("minted:t", "abcdefg[")
	p := filepath.Join(t.TempDir(), "f")
	if err := WriteFile(p, []byte("abcdefg x"), 0o600); err != nil {
		t.Fatal(err)
	}
	f, err := os.OpenFile(p, os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	f.Seek(7, io.SeekStart)
	if err := os.Rename(p, p+".moved"); err != nil {
		t.Fatal(err)
	}
	NewWriter(f, SinkFile).Write([]byte("safe")) // Codex's input
	if bad, got := fileLeaks(t, p+".moved"); bad {
		t.Errorf("a write the kit could not place left a held value: %q", got)
	}
}

// faultWriter persists the first four bytes of its first write and
// reports a short write; later writes go through.
type faultWriter struct {
	b bytes.Buffer
	n int
}

func (f *faultWriter) Write(p []byte) (int, error) {
	if f.n++; f.n == 1 {
		f.b.Write(p[:4])
		return 4, io.ErrShortWrite
	}
	return f.b.Write(p)
}

// Class file-write-error-state-loss: after a failed or short write the
// stream is unknown, and every later write is withheld.
func TestKit8ShortWriteMarksTheStreamUnknown(t *testing.T) {
	resetForTest(Declarations{})
	Hold("minted:t", "secret!!")
	fw := &faultWriter{}
	w := NewWriter(fw, SinkFile)
	w.Write([]byte("secrX")) // Codex's input
	w.Write([]byte("et!!"))
	if got := fw.b.Bytes(); withheld(got) {
		t.Errorf("a write after a short write completed the credential: %q", got)
	}
}

// Class stream-context-truncation: a JSON document open in any reading
// keeps its head in the context, or the stream is lost.
func TestKit8OpenDocumentAfterAQuote(t *testing.T) {
	resetForTest(Declarations{})
	Hold("minted:t", "secret!!")
	writes := []string{"x\"{\"a\":\"secr\",\"", strings.Repeat("ordinary output ", 100), "\":0,\"b\":\"et!!\"}"} // Codex's input
	p := filepath.Join(t.TempDir(), "f")
	a, err := OpenFile(p, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range writes {
		a.Write([]byte(s))
	}
	a.Close()
	if bad, got := fileLeaks(t, p); bad {
		t.Errorf("a document whose head was cut joined the credential in the file: %q", got[:40]+"..."+got[len(got)-20:])
	}
	// The same through a stream that is not a file (stdout's context).
	var out bytes.Buffer
	w := NewWriter(&out, SinkFile)
	for _, s := range writes {
		w.Write([]byte(s))
	}
	if withheld(out.Bytes()) {
		t.Errorf("a document whose head was cut joined the credential on a stream")
	}
}

// Class replacement-context-unchecked: the context after a notice is what
// the sink holds, its open documents included, and is not cut without
// them.
func TestKit8NoticeInsideAnOpenDocument(t *testing.T) {
	resetForTest(Declarations{})
	Hold("minted:t", "secret!!")
	writes := []string{"z\":0} secret!! ", strings.Repeat("ordinary output ", 100), "\":0,\"b\":\"et!!\"}"} // Codex's input
	p := filepath.Join(t.TempDir(), "f")
	if err := WriteFile(p, []byte("{\"a\":\"secr\",\""), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, s := range writes {
		AppendFile(p, []byte(s), 0o600)
	}
	if bad, got := fileLeaks(t, p); bad {
		t.Errorf("the notice inside an open document let the credential join in the file: %q", got[:60]+"..."+got[len(got)-20:])
	}
	var out bytes.Buffer
	w := NewWriter(&out, SinkFile)
	for _, s := range append([]string{"{\"a\":\"secr\",\""}, writes...) {
		w.Write([]byte(s))
	}
	if withheld(out.Bytes()) {
		t.Errorf("the notice inside an open document let the credential join on a stream")
	}
}

// Class code-list-token-edges: a span that starts or ends inside a token
// is compared over every token the Numbers spellings of a held all-digit
// value can reach, not a window of eight.
func TestKit8CodeListNumberSpellingEdges(t *testing.T) {
	holding(t, "10000000")
	zeros := strings.Repeat("48,", 40)
	for _, u := range []string{
		"1049,101," + zeros + "55",    // Codex's input: starts inside 1049, at 049 ('1')
		"49,101," + zeros + "557",     // ends inside 557, at 55 ('7')
		"1049,101," + zeros + "557",   // both
		"1046,49,101," + zeros + "56", // starts at 046 ('.'): .1e0...08
		"7,1049,101," + zeros + "557,9",
	} {
		if !withheld([]byte(u)) {
			t.Errorf("%.40q... released (a span reads 1e0...07, 10000000)", u)
		}
	}
	for _, u := range []string{
		"1049,101," + zeros + "56", // 1e0...08: another number
		"1049,101," + zeros,        // no exponent digit
	} {
		if withheld([]byte(u)) {
			t.Errorf("%.40q... withheld", u)
		}
	}
}

// Class modal-charset-pruning, kit7rv round 3: hex, then ISO-2022-JP, then
// percent-encoding, then the join. Four decodings, a new member of a class
// rounds 1 and 2 of this session fixed: advisory under section 6, item 10.
func TestKit8ModalReadingFourDecodings(t *testing.T) {
	t.Skip("contract section 6, item 10: a modal charset reading that a joined value needs only after two more decodings")
	holding(t, "secret!!")
	if !withheld([]byte("253742221b244224221b2842223a302c2261223a2273656372222c2262223a2265742121227d")) { // Codex's input
		t.Error("released")
	}
}
