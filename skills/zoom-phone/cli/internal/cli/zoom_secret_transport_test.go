// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"bytes"
	"compress/gzip"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestZoomSecretRedactTransport(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/phone/common_areas/activation_codes":
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, `{"common_areas_activation_codes":[{"common_area_id":"ca1","activation_code":"AC-SECRET-1"}],"next_page_token":"keep"}`)
		case "/v2/phone/recording/download/f1":
			w.Header().Set("Content-Type", "audio/mpeg")
			_, _ = io.WriteString(w, "ID3\x03\x00not-json-content")
		}
	}))
	defer srv.Close()
	c := &http.Client{Transport: zoomSecretRedactTransport{base: http.DefaultTransport}}

	resp, err := c.Get(srv.URL + "/phone/common_areas/activation_codes")
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if strings.Contains(string(body), "AC-SECRET-1") || !strings.Contains(string(body), `"next_page_token":"keep"`) || !strings.Contains(string(body), "ca1") {
		t.Fatalf("activation code not redacted or other fields lost: %s", body)
	}
	if resp.ContentLength != int64(len(body)) {
		t.Fatalf("content length %d != body %d", resp.ContentLength, len(body))
	}

	resp, err = c.Get(srv.URL + "/v2/phone/recording/download/f1")
	if err != nil {
		t.Fatal(err)
	}
	body, _ = io.ReadAll(resp.Body)
	resp.Body.Close()
	if !strings.Contains(string(body), "not-json-content") {
		t.Fatalf("binary response altered: %s", body)
	}
}

// The client asks for gzip itself, so compressed JSON must be decoded and
// redacted at the transport (path-collision case from the Codex review).
func TestZoomSecretRedactTransportGzip(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var buf bytes.Buffer
		zw := gzip.NewWriter(&buf)
		_, _ = io.WriteString(zw, `{"common_areas_activation_codes":[{"common_area_id":"ca1","activation_code":"AC-GZIP-SECRET"}]}`)
		_ = zw.Close()
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Content-Encoding", "gzip")
		_, _ = w.Write(buf.Bytes())
	}))
	defer srv.Close()
	c := &http.Client{Transport: zoomSecretRedactTransport{base: &http.Transport{DisableCompression: true}}}
	req, _ := http.NewRequest(http.MethodGet, srv.URL+"/phone/common_areas/activation_codes", nil)
	req.Header.Set("Accept-Encoding", "gzip")
	resp, err := c.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.Header.Get("Content-Encoding") != "" {
		t.Fatal("Content-Encoding left set on a decoded body")
	}
	if strings.Contains(string(body), "AC-GZIP-SECRET") || !strings.Contains(string(body), "ca1") {
		t.Fatalf("gzip body not decoded and redacted: %q", body)
	}
}

// Bodies the client normalizes before parsing (UTF-8 BOM, XSSI prefix) and
// escaped key names must still be redacted; unparseable JSON is refused.
func TestZoomSecretRedactTransportNormalizedBodies(t *testing.T) {
	cases := map[string]string{
		"/bom":      "\xEF\xBB\xBF{\"activation_code\":\"BOM-SECRET\"}",
		"/escaped":  `{"\u0061ctivation_code":"ESCAPED-SECRET"}`,
		"/xssi":     ")]}'\n{\"pin\":\"XSSI-SECRET\"}",
		"/broken":   `{"activation_code":"BROKEN-SECRET"`,
		"/trailer":  `{"pin":"A"} {"pin":"TRAILER-SECRET"}`,
		"/garbage":  `prefix {"activation_code":"GARBAGE-SECRET"}`,
		"/spacebom": " \xEF\xBB\xBF{\"activation_code\":\"SPACEBOM-SECRET\"}",
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, cases[r.URL.Path])
	}))
	defer srv.Close()
	c := &http.Client{Transport: zoomSecretRedactTransport{base: http.DefaultTransport}}
	for _, path := range []string{"/bom", "/escaped", "/xssi"} {
		resp, err := c.Get(srv.URL + path)
		if err != nil {
			t.Fatalf("%s: %v", path, err)
		}
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		if strings.Contains(string(body), "SECRET") {
			t.Fatalf("%s: secret passed through: %q", path, body)
		}
	}
	for _, path := range []string{"/broken", "/trailer", "/garbage", "/spacebom"} {
		resp, err := c.Get(srv.URL + path)
		if err != nil && strings.Contains(err.Error(), "SECRET") {
			t.Fatalf("%s: refusal leaked the body: %v", path, err)
		}
		if err == nil {
			body, _ := io.ReadAll(resp.Body)
			resp.Body.Close()
			t.Fatalf("%s: unparseable JSON passed through: %q", path, body)
		}
	}
}

// Codex review r1 (class secret-redaction: the response's own Content-Type
// decides whether redaction runs). A JSON endpoint answering with a binary
// media type must not skip redaction: the client would base64-wrap the body
// and hand the SIP password to the caller. Binary bodies pass unaltered only
// for Zoom's documented download endpoints; anything that looks like JSON is
// redacted (or refused when it does not parse) whatever its label.
func TestZoomSecretRedactTransportBinaryLabelOnJSONEndpoint(t *testing.T) {
	sipJSON := `{"id":"d1","provision":{"sip_accounts":[{"password":"SIP-SECRET"}]}}`
	utf16 := func(s string) string {
		var b strings.Builder
		b.WriteString("\xFF\xFE")
		for _, r := range s {
			b.WriteByte(byte(r))
			b.WriteByte(0)
		}
		return b.String()
	}
	type reply struct{ ct, body string }
	routes := map[string]reply{
		"/v2/phone/devices/d1":                 {"application/octet-stream", sipJSON},
		"/v2/phone/devices/d2":                 {"application/vnd.zoom.device", sipJSON},
		"/v2/phone/devices/d3":                 {"application/octet-stream", utf16(sipJSON)},
		"/v2/phone/devices/d4":                 {"application/x-zoom", `{"pin":"PIN-SECRET"`},
		"/v2/phone/devices/d5":                 {"image/png", sipJSON},
		"/v2/phone/devices/d6":                 {"text/plain", `{"activation_code":"AC-SECRET"`},
		"/v2/phone/recording/download/f1":      {"audio/mpeg", "ID3\x03\x00\x00\x00binary-audio"},
		"/v2/phone/voice_mails/download/f2":    {"application/octet-stream", sipJSON},
		"/v2/phone/fax/logs/l1/file/f3":        {"application/pdf", "%PDF-1.7 fax"},
		"/v2/phone/recording/download/..%2Fx":  {"application/octet-stream", utf16(sipJSON)},
		"/v2/phone/recording/download/f4/more": {"application/octet-stream", utf16(sipJSON)},
		"/v2/phone/devices/d7":                 {"application/xml", "<device><password>SIP-SECRET</password></device>"},
		"/v2/phone/devices/d8":                 {"text/plain", "password=SIP-SECRET"},
		"/v2/phone/devices/d9":                 {"text/html", "<!doctype html><html><title>Sign in</title></html>"},
		"/v2/phone/devices/e1":                 {"text/plain", "Bad Request"},
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rep := routes[r.URL.EscapedPath()]
		w.Header().Set("Content-Type", rep.ct)
		if strings.HasSuffix(r.URL.Path, "/e1") {
			w.WriteHeader(http.StatusBadRequest)
		}
		_, _ = io.WriteString(w, rep.body)
	}))
	defer srv.Close()
	c := &http.Client{Transport: zoomSecretRedactTransport{base: http.DefaultTransport}}
	get := func(path string) (string, error) {
		t.Helper()
		resp, err := c.Get(srv.URL + path)
		if err != nil {
			return "", err
		}
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		return string(body), nil
	}
	for _, path := range []string{"/v2/phone/devices/d1", "/v2/phone/devices/d2", "/v2/phone/devices/d3", "/v2/phone/devices/d4", "/v2/phone/devices/d5", "/v2/phone/devices/d6", "/v2/phone/devices/d7", "/v2/phone/devices/d8", "/v2/phone/recording/download/..%2Fx", "/v2/phone/recording/download/f4/more"} {
		body, err := get(path)
		if err != nil {
			if strings.Contains(err.Error(), "SECRET") {
				t.Fatalf("%s: refusal leaked the body: %v", path, err)
			}
			continue
		}
		plain := strings.ReplaceAll(body, "\x00", "")
		if strings.Contains(plain, "SECRET") {
			t.Fatalf("%s: credential passed the transport under a non-JSON label: %q", path, body)
		}
	}
	if body, err := get("/v2/phone/recording/download/f1"); err != nil || body != "ID3\x03\x00\x00\x00binary-audio" {
		t.Fatalf("recording download altered: %q %v", body, err)
	}
	if body, err := get("/v2/phone/fax/logs/l1/file/f3"); err != nil || body != "%PDF-1.7 fax" {
		t.Fatalf("fax file download altered: %q %v", body, err)
	}
	// credkit-wiring: an HTML page on a 2xx is refused (the client
	// would quote its title), and a plain-text error body is withheld behind a
	// fixed JSON notice while its status still reaches the client.
	if body, err := get("/v2/phone/devices/d9"); err == nil || strings.Contains(body, "Sign in") {
		t.Fatalf("HTML page passed: %q %v", body, err)
	}
	if body, err := get("/v2/phone/devices/e1"); err != nil || strings.Contains(body, "Bad Request") || !strings.Contains(body, "zoom_phone_body_withheld") {
		t.Fatalf("plain-text error body not withheld: %q %v", body, err)
	}
	if body, err := get("/v2/phone/voice_mails/download/f2"); err != nil || strings.Contains(body, "SIP-SECRET") || !strings.Contains(body, `"d1"`) {
		t.Fatalf("JSON on a download endpoint not redacted: %q %v", body, err)
	}
}
