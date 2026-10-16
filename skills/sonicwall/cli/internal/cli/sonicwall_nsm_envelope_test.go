// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"compress/gzip"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestNSMEnvelopeFailure(t *testing.T) {
	cases := []struct {
		name   string
		body   string
		status int
		ok     bool
	}{
		{"invalid auth (observed wire)", `{"status":{"info":[{"code":"E_INVALIDAUTH","level":"error","message":"unauthenticated for invalid credentials"}],"success":false}}`, 401, true},
		{"unknown path (observed wire)", `{"status":{"info":[{"code":"E_FAIL","level":"error","message":"path /api/manager/x was not found"}],"success":false}}`, 404, true},
		{"permission", `{"status":{"info":[{"code":"E_PERMISSION_DENIED","message":"no"}],"success":false}}`, 403, true},
		{"generic failure", `{"status":{"info":[{"code":"E_FAIL","message":"bad input"}],"success":false}}`, 400, true},
		{"success envelope passes (sso token, health)", `{"status":{"info":[{"code":"E_OK","message":"HEALTH OK"}],"success":true}}`, 0, false},
		{"device array passes", `[{"serialNumber":"0040AABBCCDD","success":false}]`, 0, false},
		{"object without status passes", `{"buildVersion":"4.3.3-R1","success":false}`, 0, false},
		{"msw envelope passes", `{"content":{},"operationStatus":"Success"}`, 0, false},
		{"empty", ``, 0, false},
		{"numeric code still a failure", `{"status":{"success":false,"info":[{"code":7,"message":"unauthenticated"}]}}`, 401, true},
		{"no info at all", `{"status":{"success":false}}`, 400, true},
		{"escaped key", `{"status":{"\u0073uccess":false,"info":[{"code":"E_INVALIDAUTH"}]}}`, 401, true},
		{"BOM prefix", "\xEF\xBB\xBF" + `{"status":{"success":false,"info":[{"code":"E_INVALIDAUTH"}]}}`, 401, true},
		{"XSSI prefix", ")]}'\n" + `{"status":{"success":false,"info":[{"code":"E_FAIL","message":"x"}]}}`, 400, true},
	}
	for _, tc := range cases {
		status, ok := nsmEnvelopeFailure([]byte(tc.body))
		if status != tc.status || ok != tc.ok {
			t.Errorf("%s: got (%d,%v) want (%d,%v)", tc.name, status, ok, tc.status, tc.ok)
		}
	}
}

func TestNSMEnvelopeTransportRewritesStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/ok" {
			_, _ = io.WriteString(w, `[{"serialNumber":"0040AABBCCDD"}]`)
			return
		}
		_, _ = io.WriteString(w, `{"status":{"info":[{"code":"E_INVALIDAUTH","message":"unauthenticated for invalid credentials"}],"success":false}}`)
	}))
	defer srv.Close()
	hc := &http.Client{Transport: nsmEnvelopeTransport{base: http.DefaultTransport}}

	resp, err := hc.Get(srv.URL + "/tenants")
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != 401 || !strings.Contains(string(body), "nsm-login") || !strings.Contains(string(body), "E_INVALIDAUTH") {
		t.Fatalf("envelope: status %d body %q, want 401 with the nsm-login hint and NSM's envelope", resp.StatusCode, body)
	}

	// Failure envelope served gzip-encoded with a non-JSON content type is still caught.
	gz := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		w.Header().Set("Content-Encoding", "gzip")
		zw := gzip.NewWriter(w)
		_, _ = io.WriteString(zw, `{"status":{"info":[{"code":"E_INVALIDAUTH","message":"x"}],"success":false}}`)
		_ = zw.Close()
	}))
	defer gz.Close()
	req, _ := http.NewRequest(http.MethodGet, gz.URL+"/tenants", nil)
	req.Header.Set("Accept-Encoding", "gzip") // explicit, so net/http does not inflate it
	resp, err = hc.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != 401 {
		t.Fatalf("gzip envelope: status %d, want 401", resp.StatusCode)
	}

	resp, err = hc.Get(srv.URL + "/ok")
	if err != nil {
		t.Fatal(err)
	}
	body, _ = io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != 200 || string(body) != `[{"serialNumber":"0040AABBCCDD"}]` {
		t.Fatalf("data: status %d body %q, want 200 untouched", resp.StatusCode, body)
	}
}
