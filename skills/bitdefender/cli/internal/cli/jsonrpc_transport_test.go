// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"compress/gzip"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestJSONRPCTransportEnvelopeAndUnwrap(t *testing.T) {
	var gotPath string
	var got rpcRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		raw, _ := io.ReadAll(r.Body)
		if err := json.Unmarshal(raw, &got); err != nil {
			t.Fatalf("server got non-JSON body %q", raw)
		}
		_, _ = w.Write([]byte(`{"id":"` + got.ID + `","jsonrpc":"2.0","result":{"items":[{"id":"e1"}],"page":1}}`))
	}))
	defer srv.Close()

	hc := &http.Client{Transport: &jsonrpcTransport{base: http.DefaultTransport}}
	resp, err := hc.Post(srv.URL+"/api/v1.1/jsonrpc/network#getEndpointsList", "application/json", strings.NewReader(`{"parentId":"c1","perPage":2}`))
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(resp.Body)
	if gotPath != "/api/v1.1/jsonrpc/network" {
		t.Fatalf("path = %q, want fragment-free area URL", gotPath)
	}
	if got.JSONRPC != "2.0" || got.Method != "getEndpointsList" || got.ID == "" {
		t.Fatalf("envelope = %+v", got)
	}
	if string(got.Params) != `{"parentId":"c1","perPage":2}` {
		t.Fatalf("params = %s", got.Params)
	}
	if resp.StatusCode != 200 || string(body) != `{"items":[{"id":"e1"}],"page":1}` {
		t.Fatalf("status %d body %s, want unwrapped result", resp.StatusCode, body)
	}
}

func TestJSONRPCTransportEmptyBodySendsEmptyParams(t *testing.T) {
	var got rpcRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(raw, &got)
		_, _ = w.Write([]byte(`{"id":"` + got.ID + `","jsonrpc":"2.0","result":null}`))
	}))
	defer srv.Close()
	hc := &http.Client{Transport: &jsonrpcTransport{base: http.DefaultTransport}}
	req, _ := http.NewRequest(http.MethodPost, srv.URL+"/api/v1.0/jsonrpc/general#getApiKeyDetails", nil)
	resp, err := hc.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(resp.Body)
	if string(got.Params) != `{}` || got.Method != "getApiKeyDetails" {
		t.Fatalf("envelope = %+v params %s", got, got.Params)
	}
	if string(body) != "null" {
		t.Fatalf("body = %s, want null result", body)
	}
}

func TestJSONRPCTransportErrorMapping(t *testing.T) {
	for _, tc := range []struct {
		code int
		want int
	}{{-32001, 403}, {-32002, 404}, {-32003, 429}, {-32602, 400}, {-32000, 400}} {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte(`{"id":"1","jsonrpc":"2.0","error":{"code":` + itoa(tc.code) + `,"message":"m","data":{"details":"why"}}}`))
		}))
		hc := &http.Client{Transport: &jsonrpcTransport{base: http.DefaultTransport}}
		resp, err := hc.Post(srv.URL+"/api/v1.2/jsonrpc/incidents#getIncident", "application/json", strings.NewReader(`{"id":"x"}`))
		srv.Close()
		if err != nil {
			t.Fatal(err)
		}
		body, _ := io.ReadAll(resp.Body)
		if resp.StatusCode != tc.want || !strings.Contains(string(body), `"details":"why"`) {
			t.Fatalf("code %d: status %d body %s, want %d with details", tc.code, resp.StatusCode, body, tc.want)
		}
	}
}

func TestJSONRPCTransportPassesThroughNonRPC(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/plain" {
			t.Errorf("path = %q", r.URL.Path)
		}
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer srv.Close()
	hc := &http.Client{Transport: &jsonrpcTransport{base: http.DefaultTransport}}
	resp, err := hc.Get(srv.URL + "/plain")
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(resp.Body)
	if string(body) != `{"ok":true}` {
		t.Fatalf("body = %s", body)
	}
}

func itoa(n int) string { b, _ := json.Marshal(n); return string(b) }

func TestJSONRPCTransportGzipAndSizeLimit(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Encoding", "gzip")
		zw := gzip.NewWriter(w)
		_, _ = zw.Write([]byte(`{"id":"1","jsonrpc":"2.0","error":{"code":-32002,"message":"Resource not found"}}`))
		_ = zw.Close()
	}))
	defer srv.Close()
	req, _ := http.NewRequest(http.MethodPost, srv.URL+"/api/v1.2/jsonrpc/incidents#getIncident", strings.NewReader(`{"id":"x"}`))
	req.Header.Set("Accept-Encoding", "gzip") // explicit, so net/http does not decompress
	resp, err := (&jsonrpcTransport{base: http.DefaultTransport}).RoundTrip(req)
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 404 || !strings.Contains(string(body), "Resource not found") || resp.Header.Get("Content-Encoding") != "" {
		t.Fatalf("gzip envelope not unwrapped: status %d body %q enc %q", resp.StatusCode, body, resp.Header.Get("Content-Encoding"))
	}
}

func TestJSONRPCTransportRejectsOversizedResponses(t *testing.T) {
	old := maxRPCResponseBytes
	maxRPCResponseBytes = 64
	defer func() { maxRPCResponseBytes = old }()
	big := `{"id":"1","jsonrpc":"2.0","result":"` + strings.Repeat("x", 200) + `"}`
	for _, gz := range []bool{false, true} {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if gz {
				w.Header().Set("Content-Encoding", "gzip")
				zw := gzip.NewWriter(w)
				_, _ = zw.Write([]byte(big))
				_ = zw.Close()
				return
			}
			_, _ = w.Write([]byte(big))
		}))
		req, _ := http.NewRequest(http.MethodPost, srv.URL+"/api/v1.0/jsonrpc/general#getApiKeyDetails", strings.NewReader(`{}`))
		if gz {
			req.Header.Set("Accept-Encoding", "gzip")
		}
		_, err := (&jsonrpcTransport{base: http.DefaultTransport}).RoundTrip(req)
		srv.Close()
		if err == nil || !strings.Contains(err.Error(), "exceeds") {
			t.Fatalf("gzip=%v: err = %v, want the size cap to reject the response", gz, err)
		}
	}
}

func TestJSONRPCTransportRejectsMalformedEnvelopes(t *testing.T) {
	for name, reply := range map[string]func(id string) string{
		"no result or error": func(id string) string { return `{"id":"` + id + `","jsonrpc":"2.0"}` },
		"wrong version":      func(id string) string { return `{"id":"` + id + `","jsonrpc":"1.0","result":{}}` },
		"wrong id":           func(string) string { return `{"id":"other","jsonrpc":"2.0","result":{"items":[]}}` },
		"not an envelope":    func(string) string { return `{}` },
		"plain text":         func(string) string { return `OK` },
	} {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			var got rpcRequest
			raw, _ := io.ReadAll(r.Body)
			_ = json.Unmarshal(raw, &got)
			_, _ = w.Write([]byte(reply(got.ID)))
		}))
		hc := &http.Client{Transport: &jsonrpcTransport{base: http.DefaultTransport}}
		_, err := hc.Post(srv.URL+"/api/v1.1/jsonrpc/network#getEndpointsList", "application/json", strings.NewReader(`{}`))
		srv.Close()
		if err == nil {
			t.Errorf("%s: want the envelope rejected", name)
		}
	}
}
