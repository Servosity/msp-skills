// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
)

func TestPageTokenTransportRenamesCursorKey(t *testing.T) {
	var got url.Values
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = r.URL.Query()
	}))
	defer srv.Close()
	c := &http.Client{Transport: pageTokenTransport{base: http.DefaultTransport}}

	resp, err := c.Get(srv.URL + "/phone/users?page_size=300&page_token=abc%2B1")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if got.Get("next_page_token") != "abc+1" || got.Has("page_token") || got.Get("page_size") != "300" {
		t.Fatalf("query = %v; want next_page_token=abc+1, page_size kept, no page_token", got)
	}

	resp, err = c.Get(srv.URL + "/phone/users?next_page_token=keep")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if got.Get("next_page_token") != "keep" {
		t.Fatalf("query without page_token changed: %v", got)
	}
}
