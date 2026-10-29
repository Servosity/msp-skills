// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

//go:build unix

package credkit

import (
	"io"
	"os"
	"strings"
	"syscall"
	"testing"
)

// Class file-stream-state-reset, kit9rv round 3 (Codex's input, driven
// with the Windows keying): a Writer of the first pipe and a Writer of a
// duplicate of its handle do not complete a credential at the reader.
func TestKit9rvDuplicateHandleWithheld(t *testing.T) {
	defer func(g string) { streamGOOS = g; dropWindowsStreams() }(streamGOOS)
	dropWindowsStreams()
	streamGOOS = "windows"
	resetForTest(Declarations{})
	Hold("minted:session", "secret!!")
	r, f, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	h, err := syscall.Dup(int(f.Fd()))
	if err != nil {
		t.Fatal(err)
	}
	alias := os.NewFile(uintptr(h), "alias")
	got := make(chan string)
	go func() { b, _ := io.ReadAll(r); got <- string(b) }()
	if _, err := NewWriter(f, SinkFile).Write([]byte("secr")); err != nil {
		t.Errorf("over-withholding: the first pipe's write failed: %v", err)
	}
	NewWriter(alias, SinkFile).Write([]byte("et!!"))
	f.Close()
	alias.Close()
	if s := <-got; strings.Contains(s, "secret!!") {
		t.Errorf("a duplicate handle completed the credential: %q", s)
	}
}
