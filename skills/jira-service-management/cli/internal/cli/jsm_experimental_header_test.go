// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestExperimentalHeaderOnRequestAPIOnly(t *testing.T) {
	var got []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = append(got, r.URL.Path+"="+r.Header.Get("X-ExperimentalApi"))
	}))
	defer srv.Close()
	rt := jsmExperimentalHeader{next: http.DefaultTransport}
	for _, p := range []string{"/rest/servicedeskapi/requesttype", "/jsm/assets/workspace/w/v1/objectschema/list"} {
		req, _ := http.NewRequest(http.MethodGet, srv.URL+p, nil)
		resp, err := rt.RoundTrip(req)
		if err != nil {
			t.Fatal(err)
		}
		_ = resp.Body.Close()
		if req.Header.Get("X-ExperimentalApi") != "" {
			t.Fatal("RoundTrip mutated the caller's request")
		}
	}
	if len(got) != 2 || got[0] != "/rest/servicedeskapi/requesttype=opt-in" || got[1] != "/jsm/assets/workspace/w/v1/objectschema/list=" {
		t.Fatalf("headers sent: %v", got)
	}
}
