// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package credkit

// Regression tests for gaps found after the first review.

import (
	"io"
	"net/http"
	"testing"
)

// Contract 1: a YAML block scalar under a credential key is held, whole and
// line by line, and a JSON null is no value.
func TestContract1ConfigBlockScalarsAndNull(t *testing.T) {
	gatherString(t, Declarations{ConfigFields: []string{"private_key", "token", "refresh"}}, "c.yaml",
		"private_key: |\n  block-line-one-111\n  block-line-two-222\nport: 8080\ntoken: >-\n  folded-aaaa\n  folded-bbbb\nrefresh: &r anchored-value-333\n")
	for _, v := range []string{"block-line-one-111", "block-line-two-222", "block-line-one-111\nblock-line-two-222", "folded-aaaa folded-bbbb", "anchored-value-333"} {
		if !isHeld(v) {
			t.Errorf("%q not held", v)
		}
	}
	if isHeld("8080") || isHeld("|") || isHeld(">-") {
		t.Error("a non-credential or a block indicator was held")
	}
	gatherString(t, Declarations{StoredRecords: []string{"refresh_token"}}, "c.json", `{"refresh_token":null}`)
	if isHeld("null") {
		t.Error("a JSON null was held as a credential")
	}
	resetForTest(Declarations{ResponseFields: []string{"refresh_token"}})
	io.ReadAll(newFieldReader(stringsReader(`{"refresh_token":null,"x":"null"}`), []string{"refresh_token"}))
	if isHeld("null") {
		t.Error("a JSON null in a response was held as a credential")
	}
}

// Contract 1: a declared response field in a redirect URL is held before
// the program can read the header.
func TestContract1ResponseFieldInRedirectURL(t *testing.T) {
	c, done := responseWith(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Location", "https://app.example.com/cb#access_token=fragment-token-abcdef&state=s")
		w.Header().Set("Refresh", "0; url=https://app.example.com/cb?access_token=query-token-abcdef")
		w.WriteHeader(http.StatusOK)
	})
	defer done()
	resetForTest(Declarations{ResponseFields: []string{"access_token"}, Origins: []Origin{{Source: "*", Allowed: []string{"https://api.example.com"}}}})
	resp, err := c.Get("https://api.example.com/authorize")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	for _, v := range []string{"fragment-token-abcdef", "query-token-abcdef"} {
		if !isHeld(v) {
			t.Errorf("%q in a response header URL was not held", v)
		}
	}
}

func stringsReader(s string) io.Reader { return &stringReader{s: s} }

type stringReader struct{ s string }

func (r *stringReader) Read(p []byte) (int, error) {
	if r.s == "" {
		return 0, io.EOF
	}
	n := copy(p, r.s)
	r.s = r.s[n:]
	return n, nil
}
