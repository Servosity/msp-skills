// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package credkit

// kit8: the four file-sink rules, each enforced by a test (files.go, the
// header comment).

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

// Rule 1: one lock per path (cleaned, absolute, links resolved), and one
// stream per file whatever name reaches it.
func TestKit8OneLockAndOneStreamPerFile(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "f")
	if err := os.WriteFile(p, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	wd, _ := os.Getwd()
	t.Cleanup(func() { os.Chdir(wd) })
	os.Chdir(dir)
	os.Symlink(p, filepath.Join(dir, "link"))
	os.Mkdir(filepath.Join(dir, "sub"), 0o700)
	k := fileKey(p)
	for _, name := range []string{"f", "./f", "sub/../f", dir + "//f", "link"} {
		if fileKey(name) != k {
			t.Errorf("%s: key %s, want %s", name, fileKey(name), k)
		}
	}
	// A second name of the same file (a hard link) is the same stream.
	resetForTest(Declarations{})
	Hold("minted:t", "secret!!")
	if err := os.Link(p, filepath.Join(dir, "hard")); err != nil {
		t.Skip("no hard links here")
	}
	AppendFile("f", []byte("secr"), 0o600)
	AppendFile("hard", []byte("et!!"), 0o600)
	if bad, got := fileLeaks(t, p); bad {
		t.Errorf("two names of one file split a credential: %q", got)
	}
}

// Rule 1 under load: every helper and handle writes one file at once; the
// file never carries the credential, whatever the interleaving (run with
// -race -count=20).
func TestKit8EveryWriterOfAFileSerializes(t *testing.T) {
	resetForTest(Declarations{})
	Hold("minted:t", "secret!!")
	p := filepath.Join(t.TempDir(), "f")
	if err := WriteFile(p, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	w, err := OpenFile(p, os.O_WRONLY|os.O_APPEND, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer w.Close()
	var wg sync.WaitGroup
	for i := range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range 5 {
				switch i % 4 {
				case 0:
					AppendFile(p, []byte("secr"), 0o600)
				case 1:
					w.Write([]byte("et!!"))
				case 2:
					WriteFile(p, []byte("secr"), 0o600)
				case 3:
					if c, err := Create(p); err == nil {
						c.Write([]byte("et!!"))
						c.Close()
					}
				}
			}
		}()
	}
	wg.Wait()
	if bad, got := fileLeaks(t, p); bad {
		t.Errorf("racing writers completed the credential: %q", got)
	}
}

// Rule 2: a write after a seek lands where it was checked, gap included.
func TestKit8SeekIsReadUnderTheLock(t *testing.T) {
	resetForTest(Declarations{})
	Hold("minted:t", "secr\x00\x00et!!")
	p := filepath.Join(t.TempDir(), "f")
	w, err := Create(p)
	if err != nil {
		t.Fatal(err)
	}
	defer w.Close()
	w.Write([]byte("secr"))
	w.f.Seek(6, io.SeekStart) // the program's seek, past the end
	w.Write([]byte("et!!"))
	if bad, got := fileLeaks(t, p); bad {
		t.Errorf("a write past the end completed the credential across the gap: %q", got)
	}
	resetForTest(Declarations{})
	Hold("minted:t", "secret!!")
	w2, _ := Create(p)
	defer w2.Close()
	w2.Write([]byte("hello world"))
	w2.f.Seek(0, io.SeekStart)
	w2.Write([]byte("HELLO"))
	if b, _ := os.ReadFile(p); string(b) != "HELLO world" {
		t.Errorf("a clean write before the end = %q", b)
	}
	if off, _ := w2.f.Seek(0, io.SeekCurrent); off != 5 {
		t.Errorf("the handle after a write at 0 of 5 bytes is at %d", off)
	}
}

// Rule 3: a fault at any step of a file write leaves the stream unknown:
// that write and every later one (to any handle of the file) is withheld,
// the notice included, until the file is truncated under the locks.
func TestKit8EveryFaultMarksTheFileUnknown(t *testing.T) {
	for _, c := range []struct{ step, how string }{
		{"stat", "append"}, {"seek", "positioned"}, {"read", "before the end"},
		{"write", "append"}, {"write", "positioned"}, {"write", "before the end"},
		{"verify", "append"}, {"verify", "before the end"},
		{"truncate", "WriteFile"}, {"write", "WriteFile"}, {"close", "WriteFile"}, {"truncate", "Create"},
	} {
		t.Run(c.step+" "+c.how, func(t *testing.T) {
			resetForTest(Declarations{})
			Hold("minted:t", "secret!!")
			p := filepath.Join(t.TempDir(), "f")
			if err := WriteFile(p, []byte("start "), 0o600); err != nil {
				t.Fatal(err)
			}
			fault := errors.New("injected")
			fileFault = func(step string) error {
				if step == c.step {
					return fault
				}
				return nil
			}
			t.Cleanup(func() { fileFault = nil })
			var err error
			switch c.how {
			case "append":
				err = AppendFile(p, []byte("one "), 0o600)
			case "positioned", "before the end":
				w, _ := OpenFile(p, os.O_WRONLY, 0)
				if c.how == "positioned" {
					w.f.Seek(0, io.SeekEnd)
				}
				_, err = w.Write([]byte("one "))
				w.Close()
			case "WriteFile":
				err = WriteFile(p, []byte("one "), 0o600)
			case "Create":
				_, err = Create(p)
			}
			fileFault = nil
			if !errors.Is(err, fault) {
				t.Fatalf("the fault was not returned: %v", err)
			}
			before, _ := os.ReadFile(p)
			// Every later write is withheld, the notice included.
			err = AppendFile(p, []byte("two "), 0o600)
			if !errors.Is(err, ErrWithheld) {
				t.Errorf("a write after the fault: %v, want withheld", err)
			}
			if after, _ := os.ReadFile(p); string(after) != string(before) {
				t.Errorf("a write after the fault reached the file: %q", after)
			}
			// A truncate under the locks starts the stream over.
			if err := WriteFile(p, []byte("fresh "), 0o600); err != nil {
				t.Fatal(err)
			}
			if err := AppendFile(p, []byte("three"), 0o600); err != nil {
				t.Errorf("after a truncate: %v", err)
			}
			if b, _ := os.ReadFile(p); string(b) != "fresh three" {
				t.Errorf("after a truncate the file is %q", b)
			}
		})
	}
	// A change the kit did not make (another writer) is uncertainty too.
	resetForTest(Declarations{})
	Hold("minted:t", "secret!!")
	p := filepath.Join(t.TempDir(), "g")
	AppendFile(p, []byte("secr"), 0o600)
	f, _ := os.OpenFile(p, os.O_WRONLY|os.O_APPEND, 0)
	f.Write([]byte("X"))
	f.Close()
	if err := AppendFile(p, []byte("et!!"), 0o600); err == nil {
		t.Error("a write after an outside change went through")
	}
	if err := AppendFile(p, []byte("et!!"), 0o600); !errors.Is(err, ErrWithheld) {
		t.Errorf("the next write: %v, want withheld", err)
	}
	if b, _ := os.ReadFile(p); string(b) != "secrX" {
		t.Errorf("writes after an outside change reached the file: %q", b)
	}
}

// Rule 4: a document open past the context's limit loses the stream for
// good (it is not reset when the document might have closed), and a binary
// stretch or an open run past it withholds until it ends.
func TestKit8OpenDocumentPastTheLimitIsLost(t *testing.T) {
	resetForTest(Declarations{})
	Hold("minted:t", "secret!!")
	p := filepath.Join(t.TempDir(), "f")
	w, _ := Create(p)
	defer w.Close()
	w.Write([]byte(`{"a":"secr","`))
	w.Write([]byte(strings.Repeat("ordinary output ", 1100))) // a member name past the limit
	if _, err := w.Write([]byte(`":0,"b":"et!!"}`)); !errors.Is(err, ErrWithheld) {
		t.Errorf("the write after a document lost its head: %v, want withheld", err)
	}
	if _, err := w.Write([]byte("\nordinary\n")); !errors.Is(err, ErrWithheld) {
		t.Errorf("a lost stream released a later write: %v", err)
	}
	if bad, got := fileLeaks(t, p); bad || strings.Contains(got, Notice(SinkFile)) {
		t.Errorf("a lost stream wrote: %q", got[len(got)-40:])
	}
	// The over-withholding guard: ordinary lines stream on, and documents
	// that close keep the context short.
	w2, _ := Create(p)
	defer w2.Close()
	line := []byte(`{"id":12,"title":"Printer offline","owner":"j.doe@example.com","tags":["a","b"]}` + "\n")
	for range 400 {
		if _, err := w2.Write(line); err != nil {
			t.Fatalf("ordinary line withheld: %v", err)
		}
	}
	if n := len(w2.fs.ctx.b); n > 2*tailLen() {
		t.Errorf("closed documents kept %d bytes of context", n)
	}
}

// Rule 4, a token still being written: a document whose write ends inside
// an escape (percent, a character reference, quoted-printable, a string
// escape) is open there, so its head stays in the context for the write
// that completes the escape.
func TestKit8PartialEscapeKeepsTheDocument(t *testing.T) {
	resetForTest(Declarations{})
	Hold("minted:t", "secret!!")
	fill := strings.Repeat("ordinary output ", 70)
	for _, ws := range [][2]string{
		{"%7B%22a%22%3A%22secr%22%2C%22" + fill + "%22%3", "A0%2C%22b%22%3A%22et!!%22%7D"},
		{"&#123;&#34;a&#34;:&#34;secr&#34;,&#34;" + fill + "&#34;&#5", "8;0,&#34;b&#34;:&#34;et!!&#34;&#125;"},
		{"=7B=22a=22=3A=22secr=22=2C=22" + fill + "=22=3", "A0=2C=22b=22=3A=22et!!=22=7D"},
		{`\u007b\u0022a\u0022:\u0022secr\u0022,\u0022` + fill + `\u0022\u003`, `a0,\u0022b\u0022:\u0022et!!\u0022\u007d`},
	} {
		p := filepath.Join(t.TempDir(), "f")
		w, _ := Create(p)
		w.Write([]byte(ws[0]))
		w.Write([]byte(ws[1]))
		w.Close()
		if bad, got := fileLeaks(t, p); bad {
			t.Errorf("a document cut inside an escape joined the credential: %q", got[len(got)-40:])
		}
	}
	// The over-withholding guard: a log's brackets are no open document.
	m, _ := currentMatcher()
	if _, open := m.checkOpen([]byte("2026-11-02 [INFO] started\n2026-11-02 [WARN] slow {x}\n")); open {
		t.Error("an ordinary log line held a document open")
	}
}
