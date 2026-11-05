// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

// Hand-written (NOT generated; handfixes.json credkit-wiring): what stdin looks
// like before the credential kit reads it. Declarations.StdinFlags holds a
// --stdin body whole and line by line, so both helpers run inside
// credkit.Start, from the Files hook, before that read.

package cliutil

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"strings"
)

// CompactStdinBody puts a --stdin JSON body on one line before the kit reads it.
// A line of "{" alone would collide with public text and refuse the start, so
// a body from `jq .` would end the run. The credentials stay held: StdinFlags
// does that. Anything that is not one JSON document stays as it came.
func CompactStdinBody(args []string) {
	if !stdinFlagPresent(args) {
		return
	}
	if fi, err := os.Stdin.Stat(); err != nil || fi.Mode()&os.ModeCharDevice != 0 {
		return
	}
	data, err := io.ReadAll(os.Stdin)
	if err != nil {
		return // the kit's own read of what is left fails closed
	}
	var one bytes.Buffer
	if json.Compact(&one, data) == nil {
		data = one.Bytes()
	}
	r, w, err := os.Pipe()
	if err != nil {
		return // os.Stdin is spent, so the command reads an empty body and refuses it
	}
	go func() {
		_, _ = w.Write(data)
		_ = w.Close()
	}()
	os.Stdin = r
}

// StashedStdin is the note `feedback --stdin` reads. A feedback note is
// operator prose, not a request body: held as a credential it could only be
// withheld from the feedback ledger it is written to, so StashFeedbackStdin
// takes it before the kit's StdinFlags read and leaves the kit an empty stdin.
var (
	StashedStdin []byte
	StdinStashed bool
)

// StashFeedbackStdin reads stdin into StashedStdin and gives the process an
// empty stdin in its place.
func StashFeedbackStdin() {
	data, err := io.ReadAll(os.Stdin)
	if err != nil {
		return // the kit reads what is left and holds it
	}
	StashedStdin, StdinStashed = data, true
	if null, err := os.Open(os.DevNull); err == nil {
		os.Stdin = null
	}
}

func stdinFlagPresent(args []string) bool {
	for _, a := range args {
		name, _, _ := strings.Cut(a, "=")
		if name == "--stdin" || name == "-stdin" {
			return true
		}
	}
	return false
}
