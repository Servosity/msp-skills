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
