package fgtcloud

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestBaseURL(t *testing.T) {
	for region, host := range map[string]string{"": "api", "global": "api", "US": "usapi", "eu": "euapi", "jp": "jpapi"} {
		got, err := BaseURL(region)
		if err != nil || got != "https://"+host+".fortigate.forticloud.com/forticloudapi/v1" {
			t.Errorf("BaseURL(%q) = %q, %v", region, got, err)
		}
	}
	if _, err := BaseURL("mars"); err == nil {
		t.Error("unknown region accepted")
	}
}

type fakeCloud struct {
	passwordGrants atomic.Int32
	refreshGrants  atomic.Int32
	reject         atomic.Bool // API answers 401 to every token
}

func (f *fakeCloud) server(t *testing.T) *httptest.Server {
	mux := http.NewServeMux()
	mux.HandleFunc("/token/", func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		var body map[string]string
		_ = json.Unmarshal(b, &body)
		if body["client_id"] != "fortigatecloud" {
			t.Errorf("client_id = %q", body["client_id"])
		}
		switch body["grant_type"] {
		case "password":
			f.passwordGrants.Add(1)
			if body["password"] != "pw" {
				w.WriteHeader(http.StatusBadRequest)
				_, _ = w.Write([]byte(`{"error":"invalid_grant"}`))
				return
			}
		case "refresh_token":
			f.refreshGrants.Add(1)
		}
		_, _ = w.Write([]byte(`{"access_token":"tok","refresh_token":"ref","expires_in":3660,"token_type":"Bearer"}`))
	})
	mux.HandleFunc("/devices", func(w http.ResponseWriter, r *http.Request) {
		if f.reject.Load() || r.Header.Get("Authorization") != "Bearer tok" {
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte(`{"error":"invalid_client"}`))
			return
		}
		off := r.URL.Query().Get("offset")
		if off == "0" {
			_, _ = w.Write([]byte(`[{"sn":"FGT60F1"},{"sn":"FGT60F2"}]`))
			return
		}
		_, _ = w.Write([]byte(`{"devices":[{"sn":"FGT60F3"}]}`))
	})
	return httptest.NewServer(mux)
}

func newTestClient(t *testing.T, srv *httptest.Server, password string) *Client {
	t.Helper()
	// The password-grant guard is process-wide per IAM user, so each test (and
	// each password) gets its own user.
	c, err := New(srv.URL, t.Name()+"-"+password, password, "", filepath.Join(t.TempDir(), "tok.json"), 5*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	c.TokenURL = srv.URL + "/token/"
	return c
}

func TestDevicesPaginatesAndCachesToken(t *testing.T) {
	f := &fakeCloud{}
	srv := f.server(t)
	defer srv.Close()
	c := newTestClient(t, srv, "pw")
	got, capped, err := c.Devices(context.Background(), 2, 5)
	if err != nil || capped || len(got) != 3 || got[2]["sn"] != "FGT60F3" {
		t.Fatalf("Devices = %v, %v, %v", got, capped, err)
	}
	if f.passwordGrants.Load() != 1 {
		t.Fatalf("password grants = %d", f.passwordGrants.Load())
	}
	info, err := os.Stat(c.CachePath)
	if err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("cache file %v mode %v", err, info)
	}
	// A second client reuses the cached token: no new login.
	c2 := newTestClient(t, srv, "pw")
	c2.CachePath = c.CachePath
	if _, err := c2.Get(context.Background(), "/devices", nil); err != nil {
		t.Fatal(err)
	}
	if f.passwordGrants.Load() != 1 {
		t.Fatalf("cached token not reused: %d grants", f.passwordGrants.Load())
	}
}

func TestExpiredTokenUsesRefresh(t *testing.T) {
	f := &fakeCloud{}
	srv := f.server(t)
	defer srv.Close()
	c := newTestClient(t, srv, "pw")
	b, _ := json.Marshal(cachedToken{User: c.User, AccessToken: "old", RefreshToken: "ref", ExpiresAt: time.Now().Add(-time.Hour)})
	_ = os.WriteFile(c.CachePath, b, 0o600)
	if _, err := c.Get(context.Background(), "/devices", nil); err != nil {
		t.Fatal(err)
	}
	if f.refreshGrants.Load() != 1 || f.passwordGrants.Load() != 0 {
		t.Fatalf("refresh=%d password=%d", f.refreshGrants.Load(), f.passwordGrants.Load())
	}
}

func TestNeverLoopsPasswordGrant(t *testing.T) {
	f := &fakeCloud{}
	f.reject.Store(true)
	srv := f.server(t)
	defer srv.Close()
	c := newTestClient(t, srv, "pw")
	_, err := c.Get(context.Background(), "/devices", nil)
	if err == nil || !strings.Contains(err.Error(), "401") {
		t.Fatalf("err = %v", err)
	}
	_, _ = c.Get(context.Background(), "/devices", nil)
	if n := f.passwordGrants.Load(); n != 1 {
		t.Fatalf("password grants = %d, want exactly 1 per process", n)
	}
	bad := newTestClient(t, srv, "wrong")
	if _, err := bad.Get(context.Background(), "/devices", nil); err == nil || !strings.Contains(err.Error(), "invalid_grant") {
		t.Fatalf("bad password err = %v", err)
	}
}

func TestRefusesRedirects(t *testing.T) {
	var hits atomic.Int32
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { hits.Add(1) }))
	defer target.Close()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, target.URL+"/steal", http.StatusTemporaryRedirect)
	}))
	defer srv.Close()
	c := newTestClient(t, srv, "pw")
	if _, err := c.Get(context.Background(), "/devices", nil); err == nil || !strings.Contains(err.Error(), "redirect") {
		t.Fatalf("redirect err = %v", err)
	}
	if hits.Load() != 0 {
		t.Fatalf("credentials followed a redirect %d time(s)", hits.Load())
	}
}

func TestLooseCacheIsIgnored(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows reports 0666 for every file")
	}
	f := &fakeCloud{}
	srv := f.server(t)
	defer srv.Close()
	c := newTestClient(t, srv, "pw")
	// A valid, unexpired token in a cache other users can read must not be used.
	b, _ := json.Marshal(cachedToken{User: c.User, AccessToken: "tok", ExpiresAt: time.Now().Add(time.Hour)})
	if err := os.WriteFile(c.CachePath, b, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(c.CachePath, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := c.Get(context.Background(), "/devices", nil); err != nil {
		t.Fatal(err)
	}
	if f.passwordGrants.Load() != 1 {
		t.Fatalf("password grants = %d; a 0644 cache was trusted", f.passwordGrants.Load())
	}
	if info, err := os.Stat(c.CachePath); err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("cache not replaced with a 0600 file: %v, %v", info, err)
	}
}

func TestCacheFileIsPrivateEvenOverLooseTemp(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "tok.json")
	// A pre-existing world-readable file at the cache path must not keep its mode.
	if err := os.WriteFile(path, []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := writeCache(path, &cachedToken{User: "u", AccessToken: "a"}); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("cache mode = %v, %v", info.Mode().Perm(), err)
	}
}

func TestListItems(t *testing.T) {
	for _, in := range []string{`[{"sn":"a"}]`, `{"data":[{"sn":"a"}]}`, `{"results":[{"sn":"a"}]}`} {
		items, err := ListItems(json.RawMessage(in))
		if err != nil || len(items) != 1 {
			t.Errorf("ListItems(%s) = %v, %v", in, items, err)
		}
	}
	if _, err := ListItems(json.RawMessage(`{"x":1}`)); err == nil {
		t.Error("ListItems accepted an object without a device array")
	}
}

func TestErrorBodiesNeverEchoSecrets(t *testing.T) {
	if got := maskSecrets(`{"error":"bad token tok-123456"}`, "tok-123456", "pw"); strings.Contains(got, "tok-123456") {
		t.Fatalf("token leaked: %s", got)
	}
}
