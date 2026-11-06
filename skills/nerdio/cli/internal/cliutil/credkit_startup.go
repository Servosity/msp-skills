// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

// Hand-written (NOT generated; handfixes.json credkit-wiring). What
// internal/credwire's Files hook reads inside credkit.Start, before anything
// prints, besides the config and credential files the kit reads itself.

package cliutil

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"nerdio-pp-cli/internal/credkit"
)

// HoldProfileStore holds the values a run profile store keeps under a declared
// credential flag (flags, spelled with their dashes) and the credentials of a
// base-url value. Only profiles.<name>.values.<flag> means anything: a
// profile's name, description and every other value are data. The kit's own
// file reader would hold every string of this nested JSON, so a profile named
// "p" or a saved --limit 7 would refuse every later start. The store is read
// token by token, because a map keeps only the last duplicate key, with a
// stack of its own so no nesting can overflow; it stops where the JSON stops.
func HoldProfileStore(data []byte, flags []string) {
	type frame struct {
		object, wantKey bool
		key             string
	}
	var stack []frame
	holdAt := func(v string) {
		var keys []string
		for _, f := range stack {
			if f.object {
				keys = append(keys, f.key)
			}
		}
		if len(keys) < 4 || !strings.EqualFold(keys[0], "profiles") || !strings.EqualFold(keys[2], "values") {
			return
		}
		flag := "--" + strings.ToLower(keys[3])
		switch {
		case flag == "--base-url" || flag == "--base_url":
			HoldURLCredentials("userinfo", v)
		case strings.TrimSpace(v) != "":
			for _, f := range flags {
				if flag == f {
					Hold(credkit.Source("flag:"+f), v)
				}
			}
		}
	}
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()
	for {
		tok, err := dec.Token()
		if err != nil {
			return
		}
		switch t := tok.(type) {
		case json.Delim:
			if t == '{' || t == '[' {
				stack = append(stack, frame{object: t == '{', wantKey: t == '{'})
				continue
			}
			stack = stack[:len(stack)-1]
		case string:
			if n := len(stack); n > 0 && stack[n-1].wantKey {
				stack[n-1].key, stack[n-1].wantKey = t, false
				continue
			}
			holdAt(t)
		case json.Number:
			holdAt(t.String())
		}
		if n := len(stack); n > 0 && stack[n-1].object {
			stack[n-1].wantKey = true
		}
	}
}

// HoldProfileStoreFile is HoldProfileStore on a file. Only a regular file is opened.
func HoldProfileStoreFile(path string, flags []string) {
	if data, err := readRegularFile(path); err == nil {
		HoldProfileStore(data, flags)
	}
}

// HoldTLSKeyFile holds the private key the MCP server would serve TLS with:
// each PEM body line and the decoded key as hex. Armor and header lines are
// not the key. Its raw bytes are not held: the kit's startup check of a binary
// value is slow and then refuses the start. Only a regular file is opened.
func HoldTLSKeyFile(path string) {
	data, err := readRegularFile(path)
	if err != nil {
		return
	}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line != "" && !strings.HasPrefix(line, "-----") && !strings.Contains(line, ":") {
			Hold("minted:tls-key", line)
		}
	}
	for rest := data; ; {
		block, more := pem.Decode(rest)
		if block == nil {
			return
		}
		Hold("minted:tls-key", hex.EncodeToString(block.Bytes))
		rest = more
	}
}

func readRegularFile(path string) ([]byte, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("%s is not a regular file", path)
	}
	return os.ReadFile(filepath.Clean(path))
}

// CompactStdinBody puts a --stdin JSON body on one line before the kit reads
// it. Declarations.StdinFlags holds a body whole and line by line, and a line
// of "{" alone collides with public text and refuses the start, so a body from
// `jq .` would end the run. The credentials stay held: StdinFlags does that.
// Anything that is not one JSON document stays as it came.
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
