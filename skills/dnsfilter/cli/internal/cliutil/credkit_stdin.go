// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

// Hand-written (handfixes.json credkit-wiring).

package cliutil

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"strings"
)

// HoldStdinToken reads the token auth set-token would read from a pipe and holds it
// before Cobra parses a flag, then gives the command the same bytes on os.Stdin. It is
// credkit's StdinFlags for a subcommand, which the kit cannot name: --stdin bodies are
// declared there. The raw command line is a superset of what the parser will mean, and
// a line that only reads as this command costs a read a terminal is spared.
func HoldStdinToken(args []string) {
	auth, setToken := false, false
	for _, a := range args {
		auth = auth || a == "auth"
		setToken = setToken || auth && a == "set-token"
	}
	if !setToken {
		return
	}
	if fi, err := os.Stdin.Stat(); err != nil || fi.Mode()&os.ModeCharDevice != 0 {
		return
	}
	data, err := io.ReadAll(os.Stdin)
	if err != nil {
		return // the command's own read reports it
	}
	Hold("stdin", string(data))
	Hold("stdin", strings.Split(string(data), "\n")...)
	refillStdin(data)
}

// CompactStdinBody puts a --stdin JSON body on one line before the kit reads it. The kit
// holds a body whole and line by line, and a line of "{" alone reads as public text and
// refuses the start, so a body from `jq .` would end every command with no message.
// Compacting changes no member and holds nothing here: StdinFlags does that. Anything
// that is not one JSON document stays as it came.
func CompactStdinBody(args []string) {
	body := false
	for _, a := range args {
		name, _, _ := strings.Cut(a, "=")
		body = body || name == "--stdin" || name == "-stdin"
	}
	if !body {
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
	refillStdin(data)
}

// refillStdin gives the program data on os.Stdin again.
func refillStdin(data []byte) {
	r, w, err := os.Pipe()
	if err != nil {
		return // an empty stdin: the command refuses it
	}
	go func() {
		_, _ = w.Write(data)
		_ = w.Close()
	}()
	os.Stdin = r
}
