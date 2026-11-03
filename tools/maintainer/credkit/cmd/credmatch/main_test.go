// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

//go:build credmatch

package main

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"slices"
	"strings"
	"testing"
)

func b64(s string) string { return base64.StdEncoding.EncodeToString([]byte(s)) }

func ask(t *testing.T, lines ...string) ([]answer, error) {
	t.Helper()
	var out bytes.Buffer
	err := serve(strings.NewReader(strings.Join(lines, "\n")), &out)
	var got []answer
	dec := json.NewDecoder(&out)
	for dec.More() {
		var a answer
		if e := dec.Decode(&a); e != nil {
			t.Fatal(e)
		}
		got = append(got, a)
	}
	return got, err
}

func line(held []string, unit string, req bool) string {
	hs := make([]string, len(held))
	for i, h := range held {
		hs[i] = b64(h)
	}
	b, _ := json.Marshal(request{Held: hs, Unit: b64(unit), Request: req})
	return string(b)
}

func TestVerdicts(t *testing.T) {
	const key = "clsKa1B2c3D4.e5F+g6H/i7J=k8L"
	cases := []struct {
		name, unit string
		held       []string
		req        bool
		want       answer
	}{
		{"whole value", "token: " + key, []string{key}, false, answer{Verdict: "leak", Clause: "2(a)"}},
		{"an 8-character piece", "x a1B2c3D4 y", []string{key}, false, answer{Verdict: "leak", Clause: "2(b)"}},
		{"base64 of the value, decoded by the kit", b64(key), []string{key}, false, answer{Verdict: "leak", Clause: "2(a)"}},
		{"nothing held", key, nil, false, answer{Verdict: "clean"}},
		{"ordinary output", `{"items":[{"id":1,"name":"printer"}]}`, []string{key}, false, answer{Verdict: "clean"}},
		{"a short value inside a word is no token", "xq7Zx", []string{"q7Z"}, false, answer{Verdict: "clean"}},
		{"a short value as a token", "a q7Z b", []string{"q7Z"}, false, answer{Verdict: "leak", Clause: "2(a)"}},
		{"a short value anywhere in a request", "GET /xq7Zx", []string{"q7Z"}, true, answer{Verdict: "leak", Clause: "2(c)"}},
	}
	for _, c := range cases {
		got, err := ask(t, line(c.held, c.unit, c.req))
		if err != nil || len(got) != 1 || got[0].Verdict != c.want.Verdict || got[0].Clause != c.want.Clause {
			t.Errorf("%s: got %v (err %v), want %v", c.name, got, err, c.want)
		}
	}
}

func TestOneAnswerPerLineInOrder(t *testing.T) {
	got, err := ask(t, line([]string{"q7Z"}, "q7Z", false), line([]string{"q7Z"}, "none", false))
	if err != nil || len(got) != 2 || got[0].Verdict != "leak" || got[1].Verdict != "clean" {
		t.Fatalf("got %v, %v", got, err)
	}
}

func TestPublicText(t *testing.T) {
	b, _ := json.Marshal(request{Public: []string{b64("true"), b64("null"), b64("clsK-not-public-1"), b64("HTTP/1.1")}})
	got, err := ask(t, string(b))
	want := []bool{true, true, false, true}
	if err != nil || len(got) != 1 || !slices.Equal(got[0].Public, want) {
		t.Fatalf("got %v (err %v), want %v", got, err, want)
	}
}

func TestUnreadableLineFails(t *testing.T) {
	for _, bad := range []string{"{not json", `{"held":["%%%"],"unit":""}`, `{"held":[],"unit":"%%%"}`, `{"public":["%%%"]}`} {
		if _, err := ask(t, bad); err == nil {
			t.Errorf("%q: no error, so a missing verdict could read as clean", bad)
		}
	}
}

func TestStreamWrites(t *testing.T) {
	stream := func(units ...string) string {
		us := make([]string, len(units))
		for i, u := range units {
			us[i] = b64(u)
		}
		b, _ := json.Marshal(request{Held: []string{b64("482913")}, Unit: b64(strings.Join(units, "")), Units: us})
		return string(b)
	}
	got, err := ask(t, stream("482913", "7\n"), stream("4829137\n"), stream("a ", "b\n"))
	if err != nil || len(got) != 3 || got[0].Verdict != "leak" || got[1].Verdict != "clean" || got[2].Verdict != "clean" {
		t.Fatalf("a short value whole at the end of one write must be withheld, the same bytes in one write must not: %v %v", got, err)
	}
}

// A clean frame the pipe split at a pause goes past the stream's limits: the
// kit withholds its second part, and that is not a credential found (corpus8rv,
// a 69 KB MCP tools/list on the fleet run). A match after a split still is.
func TestStreamLimitIsNotALeak(t *testing.T) {
	stream := func(units ...string) string {
		us := make([]string, len(units))
		for i, u := range units {
			us[i] = b64(u)
		}
		b, _ := json.Marshal(request{Held: []string{b64("clsKmJRm2g3d.pCR+42P/hNY=FJj")}, Unit: b64(strings.Join(units, "")), Units: us})
		return string(b)
	}
	frame := `{"jsonrpc":"2.0","id":null,"result":{"tools":[` + strings.Repeat(`{"name":"a_tool","description":"Output as JSON (default: false)","type":"boolean"},`, 900) + `{}]}}` + "\n"
	cut := len(frame) - 3700
	got, err := ask(t, stream(frame[:cut], frame[cut:]), stream(frame))
	if err != nil || len(got) != 2 {
		t.Fatalf("got %v, %v", got, err)
	}
	if got[0].Verdict != "exhausted" || !strings.Contains(got[0].Clause, "Streams") {
		t.Errorf("a clean frame split at a pause: want exhausted (a limit), got %+v", got[0])
	}
	if got[1].Verdict != "clean" {
		t.Errorf("the same frame in one write: want clean, got %+v", got[1])
	}
	// corpus8rv round 6: a limit does not end the check. A whole short value in
	// a later write is a leak, whichever write the limit came first in.
	short := func(units ...string) string {
		var q request
		if err := json.Unmarshal([]byte(stream(units...)), &q); err != nil {
			t.Fatal(err)
		}
		q.Held = []string{b64("482913")}
		b, _ := json.Marshal(q)
		return string(b)
	}
	got, err = ask(t, short(frame[:cut], frame[cut:], "482913", "7\n"), short(frame[:cut], frame[cut:], "ok\n"))
	if err != nil || len(got) != 2 {
		t.Fatalf("got %v, %v", got, err)
	}
	if got[0].Verdict != "leak" {
		t.Errorf("a whole short value in a write after a limit: want leak, got %+v", got[0])
	}
	if got[1].Verdict != "exhausted" {
		t.Errorf("a clean write after a limit: want exhausted, got %+v", got[1])
	}
	// The whole stream exhausting the budget does not end it either.
	zeros := strings.Repeat("\x00", 1<<16)
	got, err = ask(t, short(zeros), short(zeros, "482913", "7\n"))
	if err != nil || len(got) != 2 {
		t.Fatalf("got %v, %v", got, err)
	}
	if got[0].Verdict != "exhausted" {
		t.Fatalf("the input no longer exhausts the whole unit, so this checks nothing: %+v", got[0])
	}
	if got[1].Verdict != "leak" {
		t.Errorf("a whole short value in a write of a stream that exhausts the budget: want leak, got %+v", got[1])
	}
}

func TestAcquireStdin(t *testing.T) {
	// A declared stdin flag in the arguments: the kit reads the process's
	// stdin (finding r4-4); without the flag, stdin is not a credential.
	for _, c := range []struct {
		args []string
		held bool
	}{{[]string{"--token-stdin"}, true}, {nil, false}} {
		q := acquire{Decl: decl{StdinFlags: []string{"--token-stdin"}}, Stdin: b64("stdin-secret-271828\n")}
		for _, a := range c.args {
			q.Args = append(q.Args, b64(a))
		}
		b, _ := json.Marshal(request{Acquire: &q})
		got, err := ask(t, string(b))
		if err != nil || len(got) != 1 {
			t.Fatalf("got %v, %v", got, err)
		}
		var held []string
		for _, h := range got[0].Held {
			v, _ := base64.StdEncoding.DecodeString(h)
			held = append(held, string(v))
		}
		if slices.Contains(held, "stdin-secret-271828") != c.held {
			t.Errorf("args %q: held %q", c.args, held)
		}
	}
}

func TestAcquire(t *testing.T) {
	q := acquire{Env: []string{b64("ACME_API_KEY=clsKenvValue123456"), b64("ACME_CLIENT_ID=tenant-public-123")},
		Decl: credkitDecl()}
	add := func(resp bool, url string, hdr [][2]string, body string) {
		h := make([][2]string, len(hdr))
		for i, p := range hdr {
			h[i] = [2]string{b64(p[0]), b64(p[1])}
		}
		q.Exchanges = append(q.Exchanges, exchange{Response: resp, Method: "GET", URL: url, Header: h, Body: b64(body)})
	}
	add(false, "http://127.0.0.1:1/v1/items", [][2]string{{"Authorization", "Bearer runtime-issued-secret-123456"}}, "")
	add(true, "http://127.0.0.1:1/v1/items", [][2]string{{"X-Request-Id", "req-clsKheaderValue98765"},
		{"Set-Cookie", "sid=cookieValue-4242; Path=/"}}, `{"password":"clsKbodyValue55555","name":"printer"}`)
	b, _ := json.Marshal(request{Acquire: &q})
	got, err := ask(t, string(b))
	if err != nil || len(got) != 1 || got[0].Verdict != "held" {
		t.Fatalf("got %v, %v", got, err)
	}
	var held []string
	for _, h := range got[0].Held {
		v, _ := base64.StdEncoding.DecodeString(h)
		held = append(held, string(v))
	}
	for _, want := range []string{"clsKenvValue123456", "runtime-issued-secret-123456", "Bearer runtime-issued-secret-123456",
		"req-clsKheaderValue98765", "cookieValue-4242", "clsKbodyValue55555"} {
		if !slices.Contains(held, want) {
			t.Errorf("not held: %q (held %q)", want, held)
		}
	}
	for _, not := range []string{"tenant-public-123", "printer"} {
		if slices.Contains(held, not) {
			t.Errorf("held, but an identifier or an undeclared field: %q", not)
		}
	}
}
