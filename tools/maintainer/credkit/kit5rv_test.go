// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package credkit

// The classes the kit5rv Codex round 1 raised (.review/kit5rv-codex-r1.md),
// each with its Codex input and the class's other members.

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
)

// Class file-stream-state-reset (contract 2, 3 Streams, 5): a file written
// as a stream of writes is checked like stdout whichever call writes it, so
// every append to one file in this process, and a whole-file write before
// it, is context for the next.
func TestKit5rvFileAppendStream(t *testing.T) {
	cases := map[string]func(p string){
		"AppendFile twice (Codex)": func(p string) {
			_ = AppendFile(p, []byte("secr"), 0o600)
			_ = AppendFile(p, []byte("et!!"), 0o600)
		},
		"WriteFile then AppendFile": func(p string) {
			_ = WriteFile(p, []byte("secr"), 0o600)
			_ = AppendFile(p, []byte("et!!"), 0o600)
		},
		"two append Writers": func(p string) {
			a, _ := OpenFile(p, os.O_WRONLY|os.O_APPEND|os.O_CREATE, 0o600)
			b, _ := OpenFile(p, os.O_WRONLY|os.O_APPEND|os.O_CREATE, 0o600)
			_, _ = a.Write([]byte("secr"))
			_, _ = b.Write([]byte("et!!"))
			a.Close()
			b.Close()
		},
		"Create then AppendFile": func(p string) {
			w, _ := Create(p)
			_, _ = w.Write([]byte("secr"))
			w.Close()
			_ = AppendFile(p, []byte("et!!"), 0o600)
		},
	}
	for name, write := range cases {
		holding(t, "secret!!")
		p := filepath.Join(t.TempDir(), "out")
		write(p)
		if b, _ := os.ReadFile(p); strings.Contains(string(b), "secret!!") {
			t.Errorf("%s: the file holds the credential: %q", name, b)
		}
	}
	// A truncating open starts a new stream: no context from before it.
	holding(t, "secret!!")
	p := filepath.Join(t.TempDir(), "out")
	_ = AppendFile(p, []byte("secr"), 0o600)
	w, _ := Create(p)
	if _, err := w.Write([]byte("et!!")); err != nil {
		t.Error(err)
	}
	w.Close()
	if b, _ := os.ReadFile(p); string(b) != "et!!" {
		t.Errorf("a new file after truncation was withheld: %q", b)
	}
}

// Class charset-start-boundary-loss (contract 2(a), 3 Candidates and
// Charsets): UTF-8 and the charsets that read ASCII as itself start a
// reading at every byte offset, so a short value's left side is always a
// view boundary: it counts as a whole token when its right side is one.
// A short value with a letter or digit after it is still inside a word.
func TestKit5rvShortValueLeftEdge(t *testing.T) {
	holding(t, "abc")
	for _, u := range []string{"fabc", "x.fabc", "éabc", "fabc\n", "1abc!"} {
		if !leaks(u) {
			t.Errorf("%q: a reading from the offset of abc releases it", u)
		}
	}
	for _, u := range []string{"fabcd", "xabcx", "abc1"} {
		if leaks(u) {
			t.Errorf("%q: abc with a word character after it withheld", u)
		}
	}
	holding(t, "4711")
	for _, u := range []string{"id 94711", "94,711", "x4711", "+94711"} {
		if !leaks(u) {
			t.Errorf("%q: a reading from the offset of 4711 releases it", u)
		}
	}
	if leaks("id 47110") {
		t.Error("4711 with a digit after it withheld")
	}
}

// Class modal-charset-pruning (contract 3 Charsets, Views, Budget):
// ISO-2022-JP and HZ switch modes with escapes, so a reading from an offset
// runs in ASCII mode until the next switch. Every switch the decoder
// accepts counts (ESC ( B and ESC ( J back to ASCII, HZ's ~} ~~ and ~ at a
// line end), the reading from offset 0 keeps its prefix, an ASCII-only
// reading is a reading, and a reading that starts inside a two-byte mode
// reads its bytes as ASCII.
func TestKit5rvModalCharsets(t *testing.T) {
	holding(t, "secret!!")
	for name, u := range map[string]string{
		"JP ESC ( B (Codex)":           "secr\x1b(Bet!!",
		"JP ESC ( J (Codex)":           "secr\x1b(Jet!!",
		"JP from inside a JIS segment": "\x1b$Bsecr\x1b(Bet!!",
		"HZ ~}":                        "secr~}et!!",
		"HZ ~ at a line end":           "secr~\net!!",
		"HZ from inside a GB segment":  "~{secr~}et!!",
	} {
		if !leaks(u) {
			t.Errorf("%s: %q released", name, u)
		}
	}
	if leaks("plain \x1b[31mred\x1b[0m and ~/path text") {
		t.Error("ordinary text with ANSI colors and a tilde withheld")
	}
}

// Class joined-values-incomplete (contract 3 Views, Joined values): a JSON
// line of a view is a document whatever the rest of the view holds, so a
// binary view's JSON lines are joined whole, not as the runs of its
// relevant characters.
func TestKit5rvJoinedInBinaryView(t *testing.T) {
	holding(t, "secret!!")
	for name, u := range map[string]string{
		"Codex":                   "\x00\n{\"☃\":0,\"a\":\"secr\",\"b\":\"et!!\"}\n",
		"array line":              "\x01\x02\n[\"secr\",7,\"et!!\"]",
		"irrelevant in the value": "\x00\n{\"a\":\"☃\",\"b\":\"secr\",\"c\":\"et!!\"}",
	} {
		if !leaks(u) {
			t.Errorf("%s: %q released", name, u)
		}
	}
}

// Class config-reader-drops-values (contract 1): the scalar on the line
// after a key's node properties is read as YAML reads any scalar, so its
// quotes end it and a comment after it is not part of it.
func TestKit5rvTaggedNextLineScalar(t *testing.T) {
	for _, tc := range []struct{ content, value string }{
		{"token: !!str\n  '?!?' # note\n", "?!?"}, // Codex's input
		{"token: !!str\n  \"?!?\" # note\n", "?!?"},
		{"token: !!str\n  ?!? # note\n", "?!?"},
		{"token: &a\n  '?!?' # note\n", "?!?"},
		{"token: !!str\n  'it''s' # note\n", "it's"},
	} {
		gatherString(t, Declarations{ConfigFields: []string{"token"}}, "c.yaml", tc.content)
		if !isHeld(tc.value) {
			t.Errorf("%q: %q was dropped", tc.content, tc.value)
		}
	}
}

// Class credential-flag-spelling-missed (contract 1, startup timing): a
// declared flag is read in every spelling a Go flag parser accepts, one
// dash or two, so "-token=v" and "-token v" are held for "--token".
func TestKit5rvFlagSpellings(t *testing.T) {
	for _, tc := range []struct {
		flag string
		args []string
	}{
		{"--token", []string{"cli", "-token=secret!!"}}, // Codex's input
		{"--token", []string{"cli", "-token", "secret!!"}},
		{"-token", []string{"cli", "--token=secret!!"}},
		{"-token", []string{"cli", "--token", "secret!!"}},
		{"-t", []string{"cli", "--t=secret!!"}},
		{"-t", []string{"cli", "--t", "secret!!"}},
	} {
		if !slices.Contains(flagValues(tc.args, tc.flag), "secret!!") {
			t.Errorf("%s %q: the value was not held", tc.flag, tc.args)
		}
	}
	if !present([]string{"cli", "-stdin"}, "--stdin") {
		t.Error("-stdin not seen for a declared --stdin")
	}
}

// Class stdout-write-unit-splitting (contract 6, item 9; owner amendment
// 2026-11-01): Codex's kit5rv round-1 input is one write(2) to stdout of
// 128 KiB of spaces and "secret!!" whose writer is descheduled for longer
// than unitIdle after the pump drained the pipe's first 64 KiB. No pipe
// reports where one write ends, so the pump sees the pause and the two parts
// are two writes of the stream: the clean part is released, the part that
// completes the credential is withheld. Here the pause is a sleep between
// the two parts, which is what the pump sees either way.
func TestKit6StdoutWriteSplitAtPause(t *testing.T) {
	t.Skip("tracked, never blocking: contract section 6 item 9 (owner amendment 2026-11-01)")
	resetForTest(Declarations{})
	Hold("minted:t", "secret!!")
	startedOnce.Do(func() { close(started) })
	inR, inW, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	outR, outW, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	p := &pump{fd: 1, sink: SinkStdout, in: inR, out: outW, done: make(chan struct{})}
	go p.run()
	got := make(chan []byte)
	go func() { b, _ := io.ReadAll(outR); got <- b }()
	inW.Write(bytes.Repeat([]byte(" "), 128<<10))
	time.Sleep(10 * unitIdle)
	inW.Write([]byte("secret!!"))
	inW.Close()
	<-p.done
	outW.Close()
	out := <-got
	if bytes.Contains(out, []byte("secret!!")) {
		t.Fatal("the credential was released whole")
	}
	if bytes.Contains(out, bytes.Repeat([]byte(" "), 8)) {
		t.Errorf("part of one write was released before the part that completes the credential: %d bytes", len(out))
	}
}
