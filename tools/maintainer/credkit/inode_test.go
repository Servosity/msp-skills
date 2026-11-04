// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package credkit

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
)

// dnsfilter's journal on Linux: the kit wrote a file, the file was deleted,
// and a new file outside the kit got its inode number. The kit took the new
// file for the old one, whose end it remembered, and refused every write to it.
// Linux (ext4, tmpfs) hands a freed inode out again at once; APFS does not, so
// this one runs where the reuse happens and says so where it does not.
func TestInodeReuseIsANewFile(t *testing.T) {
	resetForTest(Declarations{})
	dir := t.TempDir()
	old := filepath.Join(dir, "old.jsonl")
	if err := AppendFile(old, []byte(strings.Repeat(`{"entry":"an earlier segment"}`+"\n", 20)), 0o600); err != nil {
		t.Fatal(err)
	}
	fi, err := os.Stat(old)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(old); err != nil {
		t.Fatal(err)
	}
	var reused string
	for i := range 200 {
		p := filepath.Join(dir, fmt.Sprintf("new-%03d.jsonl", i))
		if err := os.WriteFile(p, []byte("{\"torn\":"), 0o600); err != nil { // outside the kit
			t.Fatal(err)
		}
		if nfi, err := os.Stat(p); err == nil && os.SameFile(fi, nfi) {
			reused = p
			break
		}
	}
	if reused == "" {
		t.Skip("this file system did not hand the inode out again (macOS); the hard-link test covers the same state")
	}
	if err := AppendFile(reused, []byte("\n{\"entry\":\"today\"}\n"), 0o600); err != nil {
		t.Fatalf("an append to a new file that reuses a deleted file's inode: %v", err)
	}
	if b, _ := os.ReadFile(reused); string(b) != "{\"torn\":\n{\"entry\":\"today\"}\n" {
		t.Errorf("the file is %q", b)
	}
}

// The same state on any platform: the kit wrote a file by one name, that name
// is gone, and the file is now smaller under another name, written outside the
// kit while the kit held no handle to it.
func TestAStateWhoseNameIsGoneIsNotKept(t *testing.T) {
	resetForTest(Declarations{})
	dir := t.TempDir()
	a, b := filepath.Join(dir, "a"), filepath.Join(dir, "b")
	if err := AppendFile(a, []byte(strings.Repeat("an earlier line\n", 20)), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Link(a, b); err != nil {
		t.Skip(err)
	}
	if err := os.Remove(a); err != nil {
		t.Fatal(err)
	}
	f, err := os.OpenFile(b, os.O_WRONLY|os.O_TRUNC, 0)
	if err != nil {
		t.Fatal(err)
	}
	f.WriteString("short\n")
	f.Close()
	if err := AppendFile(b, []byte("next\n"), 0o600); err != nil {
		t.Fatalf("append: %v", err)
	}
	if got, _ := os.ReadFile(b); string(got) != "short\nnext\n" {
		t.Errorf("the file is %q", got)
	}
}

// Between kit calls no kit handle holds the file, so the kit reads it as it
// finds it: an outside shrink or rewrite is the file now, and what it holds is
// the context of the next write.
func TestAFileNoKitHandleHoldsIsReadAsFound(t *testing.T) {
	resetForTest(Declarations{})
	Hold("minted:t", "secret!!")
	completed := func(p string) bool {
		b, _ := os.ReadFile(p)
		return strings.Contains(string(b), "secret!!")
	}
	// A withheld write leaves its notice and reports success; the file says.
	p := filepath.Join(t.TempDir(), "f")
	AppendFile(p, []byte("0123456789"), 0o600)
	if err := os.WriteFile(p, []byte("xxsecr"), 0o600); err != nil { // outside the kit
		t.Fatal(err)
	}
	if err := AppendFile(p, []byte("et!!\n"), 0o600); err != nil && !errors.Is(err, ErrWithheld) {
		t.Errorf("an append to a file rewritten outside: %v", err)
	}
	if completed(p) {
		t.Error("a credential was completed across an outside rewrite")
	}
	if b, _ := os.ReadFile(p); !strings.HasPrefix(string(b), "xxsecr") || len(b) == len("xxsecr") {
		t.Errorf("the append did not land after what the file holds: %q", b)
	}
	// A file another run left is read the same way the first time.
	q := filepath.Join(t.TempDir(), "g")
	os.WriteFile(q, []byte("left by an earlier run: secr"), 0o600)
	resetFileStatesForTest()
	AppendFile(q, []byte("et!!\n"), 0o600)
	if completed(q) {
		t.Error("a credential was completed across a file first seen")
	}
	// An unchanged file keeps the kit's own context across calls.
	r := filepath.Join(t.TempDir(), "h")
	AppendFile(r, []byte("one secr"), 0o600)
	AppendFile(r, []byte("et!! two\n"), 0o600)
	if completed(r) {
		t.Error("a credential was completed across two calls")
	}
}

// A file a kit handle still holds keeps rule 3: another writer's append is
// accepted, a shrink fails closed.
func TestAFileAKitHandleHoldsStaysFailClosed(t *testing.T) {
	resetForTest(Declarations{})
	p := filepath.Join(t.TempDir(), "log")
	w, err := OpenFile(p, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	defer w.Close()
	w.Write([]byte("0123456789\n"))
	f, _ := os.OpenFile(p, os.O_WRONLY|os.O_APPEND, 0)
	f.WriteString("another writer\n")
	f.Close()
	if _, err := w.Write([]byte("after growth\n")); err != nil {
		t.Errorf("after another writer's append: %v", err)
	}
	if err := os.Truncate(p, 4); err != nil {
		t.Fatal(err)
	}
	if _, err := w.Write([]byte("x")); err == nil {
		t.Error("a write after an outside shrink went through")
	}
	// A second kit call while the handle is open does not reset it.
	if err := AppendFile(p, []byte("y"), 0o600); err == nil {
		t.Error("an AppendFile while a kit handle holds the shrunk file went through")
	}
	if b, _ := os.ReadFile(p); string(b) != "0123" {
		t.Errorf("writes after a shrink reached the file: %q", b)
	}
}

// The table stays bounded however many files a process writes and deletes.
func TestFileStatesStayBounded(t *testing.T) {
	resetForTest(Declarations{})
	resetFileStatesForTest()
	dir := t.TempDir()
	for i := range 3 * maxFileStates {
		p := filepath.Join(dir, fmt.Sprintf("seg-%05d", i))
		if err := AppendFile(p, []byte("a line\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		if i%2 == 0 {
			os.Remove(p)
		}
	}
	fileStates.Lock()
	n := len(fileStates.list)
	fileStates.Unlock()
	if n > maxFileStates {
		t.Errorf("%d file states kept, want at most %d", n, maxFileStates)
	}
}

func resetFileStatesForTest() {
	fileStates.Lock()
	fileStates.list = nil
	fileStates.Unlock()
}

// Codex r1: a new state is pruned only after it is registered, so a full table
// does not drop the state a writer just opened.
func TestAFullTableKeepsTheStateItAdds(t *testing.T) {
	resetForTest(Declarations{})
	resetFileStatesForTest()
	Hold("minted:t", "secret!!")
	dir := t.TempDir()
	for i := range maxFileStates {
		if err := WriteFile(filepath.Join(dir, fmt.Sprintf("kept-%05d", i)), []byte("x\n"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	p := filepath.Join(dir, "new")
	a, err := OpenFile(p, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close()
	b, err := OpenFile(p, os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	defer b.Close()
	a.Write([]byte("secr"))
	b.Write([]byte("et!!"))
	if got, _ := os.ReadFile(p); strings.Contains(string(got), "secret!!") {
		t.Errorf("two writers of one file kept two streams: %q", got)
	}
}

// Codex r1: reading a file as it finds it reads its tail, not the whole file.
func TestReadingAFileAsFoundReadsOnlyItsTail(t *testing.T) {
	resetForTest(Declarations{})
	p := filepath.Join(t.TempDir(), "big")
	line := []byte(strings.Repeat("an ordinary journal line ", 3) + "\n")
	if err := os.WriteFile(p, bytes.Repeat(line, (64<<20)/len(line)), 0o600); err != nil { // outside the kit
		t.Fatal(err)
	}
	var before, after runtime.MemStats
	runtime.ReadMemStats(&before)
	if err := AppendFile(p, []byte("next\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	runtime.ReadMemStats(&after)
	if d := after.TotalAlloc - before.TotalAlloc; d > 16<<20 {
		t.Errorf("an append to a 64 MiB file allocated %d MiB", d>>20)
	}
}

// Codex r2: a second opener that takes the state's lock first reads the file as
// found itself, rather than taking the first opener's registration for it.
func TestASecondOpenerReadsAFreshState(t *testing.T) {
	resetForTest(Declarations{})
	Hold("minted:t", "secret!!")
	dir := t.TempDir()
	p, q := filepath.Join(dir, "p"), filepath.Join(dir, "q")
	if err := AppendFile(p, []byte("secr"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Link(p, q); err != nil {
		t.Skip(err)
	}
	resetFileStatesForTest() // as the table evicts an idle state
	reached, release := make(chan struct{}), make(chan struct{})
	var first atomic.Bool
	stateHook = func() {
		if first.CompareAndSwap(false, true) {
			close(reached)
			<-release
		}
	}
	defer func() { stateHook = nil }()
	done := make(chan struct{})
	go func() { AppendFile(p, []byte("safe\n"), 0o600); close(done) }()
	<-reached
	AppendFile(q, []byte("et!!"), 0o600)
	close(release)
	<-done
	if got, _ := os.ReadFile(p); strings.Contains(string(got), "secret!!") {
		t.Errorf("a credential was completed: %q", got)
	}
}

// Codex r2: pruning reads no state another goroutine is writing (go test -race).
func TestPruningRacesNoWriter(t *testing.T) {
	resetForTest(Declarations{})
	resetFileStatesForTest()
	dir := t.TempDir()
	w, err := OpenFile(filepath.Join(dir, "busy"), os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	defer w.Close()
	stop := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			select {
			case <-stop:
				return
			default:
				w.Write([]byte("a line\n"))
			}
		}
	}()
	for i := range maxFileStates + 200 {
		WriteFile(filepath.Join(dir, fmt.Sprintf("f-%05d", i)), []byte("x\n"), 0o600)
	}
	close(stop)
	wg.Wait()
}
