package fortimgr

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestGetSendsReadOnlyBearerRequest(t *testing.T) {
	var got map[string]any
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/jsonrpc" || r.Method != http.MethodPost {
			t.Errorf("request %s %s", r.Method, r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer k3y" {
			t.Errorf("auth header %q", r.Header.Get("Authorization"))
		}
		b, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(b, &got)
		_, _ = w.Write([]byte(`{"id":1,"result":[{"status":{"code":0,"message":"OK"},"url":"/dvmdb/adom","data":[{"name":"acme"}]}]}`))
	}))
	defer srv.Close()
	c, err := New(srv.URL, "k3y", true, 5*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	data, err := c.Get(context.Background(), "/dvmdb/adom", map[string]any{"fields": []string{"name"}})
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != `[{"name":"acme"}]` {
		t.Fatalf("data = %s", data)
	}
	if got["method"] != "get" || got["verbose"].(float64) != 1 {
		t.Fatalf("request body = %v", got)
	}
	params := got["params"].([]any)[0].(map[string]any)
	if params["url"] != "/dvmdb/adom" || params["fields"] == nil {
		t.Fatalf("params = %v", params)
	}
}

func TestGetStatusError(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Some builds return result as an object rather than a list.
		_, _ = w.Write([]byte(`{"id":1,"result":{"status":{"code":-11,"message":"No permission for the resource"},"url":"/dvmdb/adom/x/device"}}`))
	}))
	defer srv.Close()
	c, _ := New(srv.URL, "k", true, 5*time.Second)
	_, err := c.Get(context.Background(), "/dvmdb/adom/x/device", nil)
	var se *StatusError
	if !errors.As(err, &se) || se.Code != -11 {
		t.Fatalf("err = %v", err)
	}
}

func TestRefusesRedirects(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "http://evil.example/jsonrpc", http.StatusTemporaryRedirect)
	}))
	defer srv.Close()
	c, _ := New(srv.URL, "k", true, 5*time.Second)
	if _, err := c.Get(context.Background(), "/sys/status", nil); err == nil || !strings.Contains(err.Error(), "redirect") {
		t.Fatalf("redirect err = %v", err)
	}
}

func TestNewRejectsPlainHTTPAndEmptyKey(t *testing.T) {
	if _, err := New("http://fmg.example.com", "k", false, 0); err == nil {
		t.Error("http URL accepted")
	}
	if _, err := New("https://fmg.example.com", " ", false, 0); err == nil {
		t.Error("empty key accepted")
	}
}

func TestFirmware(t *testing.T) {
	cases := []struct {
		in   map[string]any
		want string
	}{
		{map[string]any{"os_ver": "7.0", "mr": 4.0, "patch": 3.0, "build": 2573.0}, "7.4.3 build2573"},
		{map[string]any{"os_ver": 6.0, "mr": 4.0, "patch": 15.0}, "6.4.15"},
		{map[string]any{"os_ver": "7.0", "mr": 2.0, "patch": -1.0}, "7.2"},
		{map[string]any{"os_ver": "unknown"}, ""},
	}
	for _, c := range cases {
		if got := Firmware(c.in); got != c.want {
			t.Errorf("Firmware(%v) = %q want %q", c.in, got, c.want)
		}
	}
}

func TestMissingStatusAndUserinfoRejected(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"id":1,"result":[{}]}`))
	}))
	defer srv.Close()
	c, _ := New(srv.URL, "k", true, 5*time.Second)
	if _, err := c.Get(context.Background(), "/dvmdb/adom", nil); err == nil {
		t.Fatal("result without status accepted")
	}
	_, err := New("https://admin:hunter2@fmg.example.net", "k", false, 0)
	if err == nil || strings.Contains(err.Error(), "hunter2") {
		t.Fatalf("userinfo err = %v", err)
	}
}
