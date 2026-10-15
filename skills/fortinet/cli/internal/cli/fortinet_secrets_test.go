// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"compress/gzip"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"fortinet-pp-cli/internal/client"
)

func TestSecretGuard(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/config/backup") {
			_, _ = w.Write([]byte(`set psksecret "ENC keepme"`))
			return
		}
		body := `{"results":[{"name":"r1","secret":"ENC radiusXYZ","psksecret":"ENC psk\/XYZ","comment":"ENCyclopedia"}]}`
		switch {
		case strings.HasSuffix(r.URL.Path, "/brotli"):
			w.Header().Set("Content-Encoding", "br")
		case strings.Contains(r.Header.Get("Accept-Encoding"), "gzip"):
			w.Header().Set("Content-Encoding", "gzip")
			zw := gzip.NewWriter(w)
			_, _ = zw.Write([]byte(body))
			_ = zw.Close()
			return
		}
		_, _ = w.Write([]byte(body))
	}))
	defer srv.Close()
	hc := &http.Client{Transport: &secretGuard{next: http.DefaultTransport}}
	get := func(path string) (string, error) {
		req, _ := http.NewRequest(http.MethodGet, srv.URL+path, nil)
		req.Header.Set("Accept-Encoding", "gzip") // as the generated client sends it
		resp, err := hc.Do(req)
		if err != nil {
			return "", err
		}
		defer resp.Body.Close()
		b, _ := io.ReadAll(resp.Body)
		return string(b), nil
	}
	out, err := get("/api/v2/cmdb/user/radius")
	if err != nil || strings.Contains(out, "XYZ") || strings.Count(out, `"ENC (redacted)"`) != 2 || !strings.Contains(out, "ENCyclopedia") {
		t.Errorf("cmdb read = %q, %v; want both ENC values (one JSON-escaped, gzip on the wire) redacted, other strings intact", out, err)
	}
	if out, err := get("/api/v2/cmdb/brotli"); err != nil || !strings.Contains(out, "refused by fortinet-cli") {
		t.Errorf("an encoding the guard cannot inflate was not refused locally: %q, %v", out, err)
	}
	if out, err := get("/api/v2/cmdb/vpn.ipsec/phase1-interface?plain-text-password=1"); err != nil || !strings.Contains(out, "refused by fortinet-cli") {
		t.Errorf("plain-text-password request was not refused locally: %q, %v", out, err)
	}
	if out, _ := get("/api/v2/monitor/system/config/backup"); !strings.Contains(out, "keepme") {
		t.Errorf("backup body altered: %q", out)
	}
}

func TestSecretGuardBypasses(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"results":[{"name":"x","secret":"ENC leakXYZ","comment":"example: \"ENC example\" more"}]}`))
	}))
	defer srv.Close()
	hc := &http.Client{Transport: &secretGuard{next: http.DefaultTransport}}
	get := func(rawPath string) (string, error) {
		resp, err := hc.Get(srv.URL + rawPath)
		if err != nil {
			return "", err
		}
		defer resp.Body.Close()
		b, _ := io.ReadAll(resp.Body)
		return string(b), nil
	}
	// An object NAME that ends like the backup path is still a cmdb read.
	for _, p := range []string{
		"/api/v2/cmdb/user/radius/x%2Fmonitor%2Fsystem%2Fconfig%2Fbackup",
		"/api/v2/cmdb/user/radius/x/api/v2/monitor/system/config/backup",
	} {
		if out, err := get(p); err != nil || strings.Contains(out, "leakXYZ") {
			t.Errorf("%s = %q, %v; want the secret redacted", p, out, err)
		}
	}
	out, _ := get("/api/v2/cmdb/user/radius")
	if !strings.Contains(out, `"comment":"example: \"ENC example\" more"`) {
		t.Errorf("escaped ENC text inside a comment was rewritten: %q", out)
	}
	for _, q := range []string{"%70lain-text-password=1", "%70lain-text-password=1&ignored=%", "%2570lain-text-password=1", "PLAIN-TEXT-PASSWORD=1"} {
		if out, err := get("/api/v2/cmdb/user/radius?" + q); err != nil || !strings.Contains(out, "refused by fortinet-cli") {
			t.Errorf("plain-text-password request %q was sent: %q, %v", q, out, err)
		}
	}
	if out, err := get("/api/v2/cmdb/user/radius?vdom=root&filter=name%3D%3Dx%"); err != nil || strings.Contains(out, "refused by fortinet-cli") {
		t.Errorf("an ordinary query with a stray %% was refused: %q, %v", out, err)
	}
}

func TestClientHooksRefusePlainHTTP(t *testing.T) {
	for raw, ok := range map[string]bool{
		"https://fgt.example.net/api/v2": true,
		"http://127.0.0.1:8080/api/v2":   true,
		"http://fgt.example.net/api/v2":  false,
	} {
		c := &client.Client{BaseURL: raw, HTTPClient: &http.Client{Transport: http.DefaultTransport}}
		if err := ApplyClientHooks(c); (err == nil) != ok {
			t.Errorf("ApplyClientHooks(%s) err = %v, want ok=%v", raw, err, ok)
		}
	}
}

func TestRedactENCEscapings(t *testing.T) {
	cases := map[string]string{
		`{"a":"\u0045NC leakA"}`:                                  `{"a":"ENC (redacted)"}`,
		`{"a":"ENC\u0020leakB"}`:                                  `{"a":"ENC (redacted)"}`,
		`{"a": ["ENC x\/y", {"b" : "ENC z"}]}`:                    `{"a": ["ENC (redacted)", {"b" : "ENC (redacted)"}]}`,
		"{\n  \"z\": 1,\n  \"a\": \"ENC p\"\n}":                   "{\n  \"z\": 1,\n  \"a\": \"ENC (redacted)\"\n}",
		`"ENC bare"`:                                              `"ENC (redacted)"`,
		`{"c":"example: \"ENC example\" more"}`:                   `{"c":"example: \"ENC example\" more"}`,
		`{"z":2,"a":"ENCyclopedia","n":1e400}`:                    `{"z":2,"a":"ENCyclopedia","n":1e400}`,
		`<html>"ENC notjson"</html>`:                              `<html>"ENC (redacted)"</html>`,
		`{"a":"ENC lit"} trailing garbage "ENC more"`:             `{"a":"ENC (redacted)"} trailing garbage "ENC (redacted)"`,
		`{"n":1e400,"a":"\u0045NC after-big-number"}`:             `{"n":1e400,"a":"ENC (redacted)"}`,
		`{"a":"ENC x"} garbage {"b":"\u0045NC after-garbage"}`:    `{"a":"ENC (redacted)"} garbage {"b":"ENC (redacted)"}`,
		`{"error":"name "a" is invalid","psksecret":"ENC LEAK1"}`: `{"error":"name "a" is invalid","psksecret":"ENC (redacted)"}`,
		`{"results":[{"name":"a"b","psksecret":"ENC LEAK2"}]}`:    `{"results":[{"name":"a"b","psksecret":"ENC (redacted)"}]}`,
		`unterminated " near {"secret":"ENC LEAK3"}`:              `unterminated " near {"secret":"ENC (redacted)"}`,
		`<p>5" tall</p>"ENC LEAK4"`:                               `<p>5" tall</p>"ENC (redacted)"`,
		`x " {"b":"\u0045\u004eC\u0020LEAK5"}`:                    `x " {"b":"ENC (redacted)"}`,
		`{"secret":"ENC LEAK-EOF`:                                 `{"secret":"ENC (redacted)`,
		`{"secret":"\u0045NC LEAK-EOF2`:                           `{"secret":"ENC (redacted)`,
		`{"secret":"ENC LEAK-EOF3\`:                               `{"secret":"ENC (redacted)`,
		"{\"secret\":\"ENC LEAK-NL\\\nmore\"} trailing":           "{\"secret\":\"ENC (redacted)\"} trailing",
		`{"results":["a "ENC b, "ENC LEAK6"]}`:                    `{"results":["a "ENC (redacted)"ENC (redacted)"]}`,
		"\xEF\xBB\xBF{\"a\":\"\\u0045NC bom\"}":                   "\xEF\xBB\xBF{\"a\":\"ENC (redacted)\"}",
		")]}'\n{\"a\":\"ENC\\u0020xssi\"}":                        ")]}'\n{\"a\":\"ENC (redacted)\"}",
		"while(1);{\"a\":[\"\\u0045NC w\"]}":                      "while(1);{\"a\":[\"ENC (redacted)\"]}",
	}
	for in, want := range cases {
		if got := string(redactENC([]byte(in))); got != want {
			t.Errorf("redactENC(%s) = %s, want %s", in, got, want)
		}
	}
}

// fleet backup's streaming client must still time out on a device that
// accepts the TCP connection and never answers (it hung forever before).
func TestGuardedStreamingClientTimesOutOnSilentDevice(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	var held []net.Conn
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			held = append(held, conn) // never answer
		}
	}()
	c := &client.Client{HTTPClient: &http.Client{Timeout: 300 * time.Millisecond, Transport: http.DefaultTransport.(*http.Transport).Clone()}}
	guardSecrets(c)
	sc := client.StreamingHTTPClient(c.HTTPClient, c.ConfiguredTimeout())
	done := make(chan error, 1)
	go func() {
		resp, err := sc.Get("http://" + ln.Addr().String() + "/api/v2/monitor/system/config/backup")
		if err == nil {
			resp.Body.Close()
		}
		done <- err
	}()
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("silent device returned a response")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("streaming client hung on a device that never answers")
	}
}
