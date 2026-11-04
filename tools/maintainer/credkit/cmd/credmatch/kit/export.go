// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

//go:build credmatch

// This file is not built here. class_secret_corpus.py copies it into a
// temporary copy of the kit package (beside the kit's own non-test files),
// so the credmatch command reaches the kit's matcher without the kit
// exporting it to connectors. It adds no matching of its own (contract 8).

package credkit

import (
	"bytes"
	"io"
	"net/http"
	"net/url"
	"os"
	"slices"
	"strings"
)

// MatchUnit is the kit's verdict on one unit for a held set: "clean",
// "leak" with the contract clause, or "exhausted" (contract 3, Budget). A
// request is an outbound request judged by contract 2(c) alone, every value
// in held being one the request's origin may not carry.
func MatchUnit(held []string, unit []byte, request bool) (verdict, clause string) {
	var m *matcher
	if request {
		m = newRequestMatcher(held)
	} else {
		m = newMatcher(held)
	}
	switch m.check(unit) {
	case leak:
		if request {
			return "leak", "2(c)"
		}
		// 2(a) when a value is there whole (a short one as a whole token),
		// else a piece of 8 or more: 2(b).
		switch newWholeMatcher(held).check(unit) {
		case leak:
			return "leak", "2(a)"
		case exhausted:
			return "leak", "2(a) or 2(b)"
		}
		return "leak", "2(b)"
	case exhausted:
		return "exhausted", "3 (budget)"
	}
	return "clean", ""
}

// PublicAsWritten is the kit's public-text rule for a value acquired while
// running (contract 2, Public text): true when the value, as written, occurs
// in one public-text entry, so it is recorded, not held.
func PublicAsWritten(v string) bool { return inPublicText(v) }

// StreamWithheld is the kit's check of one stream (stdout, stderr), as its
// pump makes it: each unit the pump reads between pauses, checked with the
// stream's context (contract 3, Streams). It returns the first unit withheld on
// a match and true; else the first withheld at a limit and false; else -1.
func StreamWithheld(held []string, units [][]byte) (int, bool) {
	if len(held) == 0 {
		return -1, false
	}
	reg.Lock()
	reg.vals = map[string]*heldEntry{}
	reg.unknown = false
	reg.gen++
	reg.m, reg.mgen = newMatcher(held), reg.gen
	reg.Unlock()
	var st streamCtx
	first := -1 // a limit does not end the check: a later match is still a leak
	for i, u := range units {
		if len(u) > maxUnit { // the pump withholds a unit it cannot check
			if first < 0 {
				first = i
			}
			st.over(nil, false)
			continue
		}
		out, withheld := st.decide(u, SinkStdout)
		if withheld {
			if st.matched(u) {
				return i, true
			}
			if first < 0 {
				first = i
			}
		}
		st.done(u, out, withheld)
	}
	return first, false
}

// matched: a held value found in the write withheld or in the stream's
// context with it. Otherwise the kit withheld at one of its limits (a budget, a
// context it no longer has): uncertain, which it rightly withholds, and no
// credential found.
func (st *streamCtx) matched(u []byte) bool {
	m, _ := currentMatcher()
	if m.check(u) == leak {
		return true
	}
	if v, _ := m.checkOpen(append(slices.Clip(st.tail.b), u...)); v == leak {
		return true
	}
	v, _ := m.checkOpen(append(slices.Clip(st.sink.b), u...))
	return st.split && v == leak
}

// Exchange is one request a program sent or one response it received.
type Exchange struct {
	Response        bool
	Method, URL     string
	Status          int // a response's status; 0 reads as 200
	Header, Trailer [][2]string
	Body            []byte
}

// Acquire is the kit's held set for one run (contract 1): what Start holds
// from the environment and the command line (the config file named by
// --config, credential flags), then what the kit's transport holds from each
// request sent and each response received, in order. unknown: a declared
// credential source could not be read, so the kit withholds every unit.
// d.Files, when the caller sets it, names the files as a program's callback
// would. stdin
// is what the process was given on stdin: Start reads it for a declared
// stdin flag from a file standing in for os.Stdin (this command's own stdin
// is its request pipe, and is put back). mcpIn is what an MCP server read on
// stdin: its declared arguments are held frame by frame, as MCPIn holds them.
func Acquire(d Declarations, env, args []string, mcpIn, stdin []byte, ex []Exchange) (held []string, recorded int, unknown bool, err error) {
	if err := d.validate(); err != nil {
		return nil, 0, false, err
	}
	reg.Lock()
	reg.decl = d
	reg.vals = map[string]*heldEntry{}
	reg.unknown, reg.m, reg.baseURL, reg.recorded = false, nil, "", nil
	reg.headers = map[string]bool{}
	reg.gen++
	reg.startup = true
	reg.Unlock()
	os.Clearenv()
	for _, kv := range env {
		k, v, _ := strings.Cut(kv, "=")
		_ = os.Setenv(k, v)
	}
	in, err := os.CreateTemp("", "credmatch-stdin-")
	if err != nil {
		return nil, 0, false, err
	}
	defer os.Remove(in.Name())
	defer in.Close()
	if _, err := in.Write(stdin); err != nil {
		return nil, 0, false, err
	}
	if _, err := in.Seek(0, io.SeekStart); err != nil {
		return nil, 0, false, err
	}
	orig := os.Stdin
	os.Stdin = in
	gather(d, args)
	if os.Stdin != in {
		_ = os.Stdin.Close() // the pipe readStdin gave the program
	}
	os.Stdin = orig
	reg.Lock()
	reg.startup = false
	reg.Unlock()
	// The MCP server's input, frame by frame, as MCPIn holds it.
	for _, line := range bytes.SplitAfter(mcpIn, []byte("\n")) {
		if len(line) > 0 {
			holdMCP(line, false)
		}
	}
	hdr := func(kv [][2]string) http.Header {
		h := http.Header{}
		for _, p := range kv {
			h.Add(p[0], p[1])
		}
		return h
	}
	for _, x := range ex {
		u, perr := url.Parse(x.URL)
		if perr != nil {
			u = &url.URL{Opaque: x.URL}
		}
		if !x.Response {
			holdRequest(&http.Request{Method: x.Method, URL: u, Header: hdr(x.Header), Trailer: hdr(x.Trailer)}, x.Body)
			continue
		}
		st := x.Status
		if st == 0 {
			st = 200
		}
		resp := &http.Response{StatusCode: st, Header: hdr(x.Header), Trailer: hdr(x.Trailer),
			Body: io.NopCloser(bytes.NewReader(x.Body)), Request: &http.Request{Method: x.Method, URL: u}}
		o, _ := originOf(u)
		holdResponse(resp, o)
		_, _ = io.Copy(io.Discard, resp.Body)
		_ = resp.Body.Close()
	}
	records.Wait() // each record's notice is written before the next request resets the held set
	reg.Lock()
	defer reg.Unlock()
	for v := range reg.vals {
		held = append(held, v)
	}
	slices.Sort(held)
	return held, len(reg.recorded), reg.unknown, nil
}
