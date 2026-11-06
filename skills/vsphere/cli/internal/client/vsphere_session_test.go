// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-written: behavior tests for the vCenter session exchange and TLS pinning.

package client

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"vsphere-pp-cli/internal/cliutil/testenv"
	"vsphere-pp-cli/internal/config"
)

type fakeVCenter struct {
	srv          *httptest.Server
	apiLogins    atomic.Int32
	viLogins     atomic.Int32
	apiSession   atomic.Value // current valid /api session id
	viSession    atomic.Value
	sawPassword  atomic.Bool // password reached a data call
	lastViHeader atomic.Value
}

func newFakeVCenter(t *testing.T) *fakeVCenter {
	t.Helper()
	f := &fakeVCenter{}
	f.apiSession.Store("")
	f.viSession.Store("")
	mux := http.NewServeMux()
	mux.HandleFunc("/api/session", func(w http.ResponseWriter, r *http.Request) {
		u, p, ok := r.BasicAuth()
		if r.Method != http.MethodPost || !ok || u != "ro@vsphere.local" || p != "s3cret" {
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = io.WriteString(w, `{"error_type":"UNAUTHENTICATED"}`)
			return
		}
		n := f.apiLogins.Add(1)
		id := "api-" + string(rune('0'+n))
		f.apiSession.Store(id)
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(id)
	})
	mux.HandleFunc("/api/vcenter/vm", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "" {
			f.sawPassword.Store(true)
		}
		if r.Header.Get(SessionHeader) == "" || r.Header.Get(SessionHeader) != f.apiSession.Load().(string) {
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = io.WriteString(w, `{"error_type":"UNAUTHENTICATED"}`)
			return
		}
		_, _ = io.WriteString(w, `[{"vm":"vm-1","name":"web01","power_state":"POWERED_ON"}]`)
	})
	mux.HandleFunc("/sdk/vim25/8.0.1.0/ServiceInstance/ServiceInstance/content", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "" || r.Header.Get(SessionHeader) != "" {
			f.sawPassword.Store(true)
		}
		_, _ = io.WriteString(w, `{"sessionManager":{"_typeName":"ManagedObjectReference","type":"SessionManager","value":"SessionManager"},"about":{"version":"8.0.2"}}`)
	})
	mux.HandleFunc("/sdk/vim25/8.0.1.0/SessionManager/SessionManager/Login", func(w http.ResponseWriter, r *http.Request) {
		var body map[string]string
		_ = json.NewDecoder(r.Body).Decode(&body)
		if body["userName"] != "ro@vsphere.local" || body["password"] != "s3cret" {
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = io.WriteString(w, `{"_typeName":"InvalidLogin","faultstring":"Cannot complete login due to an incorrect user name or password."}`)
			return
		}
		n := f.viLogins.Add(1)
		id := "vi-" + string(rune('0'+n))
		f.viSession.Store(id)
		w.Header().Set(SessionHeader, id)
		_, _ = io.WriteString(w, `{"userName":"VSPHERE.LOCAL\\ro"}`)
	})
	mux.HandleFunc("/sdk/vim25/8.0.1.0/VirtualMachine/vm-1/snapshot", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "" {
			f.sawPassword.Store(true)
		}
		f.lastViHeader.Store(r.Header.Get(SessionHeader))
		if r.Header.Get(SessionHeader) != f.viSession.Load().(string) {
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = io.WriteString(w, `{"_typeName":"NotAuthenticated","faultstring":"The session is not authenticated."}`)
			return
		}
		_, _ = io.WriteString(w, `{"rootSnapshotList":[]}`)
	})
	f.srv = httptest.NewTLSServer(mux)
	t.Cleanup(f.srv.Close)
	return f
}

func newTestClient(t *testing.T, base, pass string) *Client {
	t.Helper()
	cfg := &config.Config{BaseURL: base, VsphereUsername: "ro@vsphere.local", VspherePassword: pass}
	c := New(cfg, 5*time.Second, 0)
	c.NoCache = true
	if err := c.InstallVsphereTransport(); err != nil {
		t.Fatalf("InstallVsphereTransport: %v", err)
	}
	return c
}

func pinFor(f *fakeVCenter) string { return CertFingerprint(f.srv.Certificate()) }

func TestSessionExchangeBothAPIs(t *testing.T) {
	testenv.Isolate(t)
	f := newFakeVCenter(t)
	t.Setenv(TLSFingerprintEnv, pinFor(f))
	c := newTestClient(t, f.srv.URL, "s3cret")
	ctx := context.Background()

	data, err := c.Get(ctx, "/api/vcenter/vm", nil)
	if err != nil || !strings.Contains(string(data), "web01") {
		t.Fatalf("vm list: %v %s", err, data)
	}
	if _, err := c.Get(ctx, "/sdk/vim25/8.0.1.0/VirtualMachine/vm-1/snapshot", nil); err != nil {
		t.Fatalf("vi snapshot: %v", err)
	}
	if f.sawPassword.Load() {
		t.Fatal("password (Authorization header) reached a data call")
	}
	if f.apiLogins.Load() != 1 || f.viLogins.Load() != 1 {
		t.Fatalf("logins api=%d vi=%d, want 1 each", f.apiLogins.Load(), f.viLogins.Load())
	}
	// A second client (a new CLI invocation) logs in again: sessions live in
	// memory only (no-credential-in-files), so nothing under the cache or
	// config dirs holds a session id.
	c2 := newTestClient(t, f.srv.URL, "s3cret")
	if _, err := c2.Get(ctx, "/api/vcenter/vm", nil); err != nil {
		t.Fatalf("second client: %v", err)
	}
	if f.apiLogins.Load() != 2 {
		t.Fatalf("second client reused a session it can only have read from disk: api logins=%d", f.apiLogins.Load())
	}
	for _, env := range []string{"HOME", "XDG_CACHE_HOME", "XDG_CONFIG_HOME", "XDG_DATA_HOME"} {
		root := os.Getenv(env)
		if root == "" {
			continue
		}
		_ = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
			if err != nil || d.IsDir() {
				return nil
			}
			b, _ := os.ReadFile(path)
			for _, id := range []string{f.apiSession.Load().(string), f.viSession.Load().(string)} {
				if id != "" && bytes.Contains(b, []byte(id)) {
					t.Errorf("%s holds a session id", path)
				}
			}
			return nil
		})
	}
}

func TestExpiredSessionsLogInAgainOnce(t *testing.T) {
	testenv.Isolate(t)
	f := newFakeVCenter(t)
	t.Setenv(TLSFingerprintEnv, pinFor(f))
	c := newTestClient(t, f.srv.URL, "s3cret")
	ctx := context.Background()
	if _, err := c.Get(ctx, "/api/vcenter/vm", nil); err != nil {
		t.Fatal(err)
	}
	if _, err := c.Get(ctx, "/sdk/vim25/8.0.1.0/VirtualMachine/vm-1/snapshot", nil); err != nil {
		t.Fatal(err)
	}
	f.apiSession.Store("expired-elsewhere") // vCenter dropped both sessions
	f.viSession.Store("expired-elsewhere")
	// The fake keeps the server-side id fixed, so make the next login win:
	// a login stores a new id, which the data handler then accepts.
	if _, err := c.Get(ctx, "/api/vcenter/vm", nil); err != nil {
		t.Fatalf("api after expiry: %v", err)
	}
	if _, err := c.Get(ctx, "/sdk/vim25/8.0.1.0/VirtualMachine/vm-1/snapshot", nil); err != nil {
		t.Fatalf("vi after expiry (NotAuthenticated fault): %v", err)
	}
	if f.apiLogins.Load() != 2 || f.viLogins.Load() != 2 {
		t.Fatalf("logins api=%d vi=%d, want 2 each", f.apiLogins.Load(), f.viLogins.Load())
	}
}

func TestBadPasswordIsAnAPIErrorWithoutRetries(t *testing.T) {
	testenv.Isolate(t)
	f := newFakeVCenter(t)
	t.Setenv(TLSFingerprintEnv, pinFor(f))
	c := newTestClient(t, f.srv.URL, "wrong")
	_, err := c.Get(context.Background(), "/api/vcenter/vm", nil)
	var apiErr *APIError
	if err == nil || !asAPIError(err, &apiErr) || apiErr.StatusCode != http.StatusUnauthorized {
		t.Fatalf("want 401 APIError, got %v", err)
	}
	if strings.Contains(err.Error(), "wrong") {
		t.Fatal("password leaked into the error")
	}
	if f.apiLogins.Load() != 0 {
		t.Fatalf("unexpected successful login")
	}
	_, err = c.Get(context.Background(), "/sdk/vim25/8.0.1.0/VirtualMachine/vm-1/snapshot", nil)
	if err == nil || !strings.Contains(err.Error(), "InvalidLogin") {
		t.Fatalf("want InvalidLogin detail, got %v", err)
	}
}

func TestPinMismatchRefusesAndMalformedPinErrors(t *testing.T) {
	testenv.Isolate(t)
	f := newFakeVCenter(t)
	t.Setenv(TLSFingerprintEnv, strings.Repeat("ab", 32))
	c := newTestClient(t, f.srv.URL, "s3cret")
	if _, err := c.Get(context.Background(), "/api/vcenter/vm", nil); err == nil || !strings.Contains(err.Error(), "fingerprint") {
		t.Fatalf("want pin mismatch, got %v", err)
	}
	if f.apiLogins.Load() != 0 {
		t.Fatal("credentials were sent to an unpinned server")
	}
	for _, bad := range []string{"not-a-pin", ",,", " , "} {
		t.Setenv(TLSFingerprintEnv, bad)
		cfg := &config.Config{BaseURL: f.srv.URL}
		if err := New(cfg, time.Second, 0).InstallVsphereTransport(); err == nil {
			t.Fatalf("malformed pin setting %q accepted", bad)
		}
	}
}

func TestCredentialsNeverLeaveTheVCenterOrigin(t *testing.T) {
	testenv.Isolate(t)
	var leaked atomic.Bool
	other := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "" || r.Header.Get(SessionHeader) != "" {
			leaked.Store(true)
		}
	}))
	defer other.Close()
	f := newFakeVCenter(t)
	t.Setenv(TLSFingerprintEnv, pinFor(f))
	c := newTestClient(t, f.srv.URL, "s3cret")
	req, _ := http.NewRequest(http.MethodGet, other.URL+"/api/vcenter/vm", nil)
	req.SetBasicAuth("ro@vsphere.local", "s3cret")
	req.Header.Set(SessionHeader, "api-1")
	resp, err := c.HTTPClient.Transport.RoundTrip(req)
	if err == nil {
		_ = resp.Body.Close()
	}
	if leaked.Load() {
		t.Fatal("credentials or session id sent to a non-vCenter origin")
	}
}

func TestPlainHTTPRefusedForRemoteHosts(t *testing.T) {
	testenv.Isolate(t)
	c := newTestClient(t, "http://vcenter.example.com", "s3cret")
	if _, err := c.Get(context.Background(), "/api/vcenter/vm", nil); err == nil || !strings.Contains(err.Error(), "https") {
		t.Fatalf("want https refusal, got %v", err)
	}
}

func TestClassifyPath(t *testing.T) {
	cases := map[string][2]string{
		"/api/vcenter/vm": {"automation", ""},
		"/sdk/vim25/8.0.1.0/HostSystem/host-1/summary": {"vi", "8.0.1.0"},
		"/sdk/vim25/../x":  {"", ""},
		"/rest/vcenter/vm": {"", ""},
		"/":                {"", ""},
	}
	for p, want := range cases {
		k, r := classifyPath(p)
		if string(k) != want[0] || r != want[1] {
			t.Errorf("classifyPath(%q) = %q,%q want %q,%q", p, k, r, want[0], want[1])
		}
	}
}

func asAPIError(err error, target **APIError) bool {
	for e := err; e != nil; {
		if a, ok := e.(*APIError); ok {
			*target = a
			return true
		}
		u, ok := e.(interface{ Unwrap() error })
		if !ok {
			return false
		}
		e = u.Unwrap()
	}
	return false
}

func TestSessionRejectedAfterFreshLoginStopsWithoutLoop(t *testing.T) {
	testenv.Isolate(t)
	var logins atomic.Int32
	mux := http.NewServeMux()
	mux.HandleFunc("/api/session", func(w http.ResponseWriter, r *http.Request) {
		logins.Add(1)
		w.WriteHeader(http.StatusCreated)
		_, _ = io.WriteString(w, `"fresh"`)
	})
	mux.HandleFunc("/api/vcenter/vm", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized) // rejects every session
	})
	srv := httptest.NewTLSServer(mux)
	defer srv.Close()
	t.Setenv(TLSFingerprintEnv, CertFingerprint(srv.Certificate()))
	c := newTestClient(t, srv.URL, "s3cret")
	_, err := c.Get(context.Background(), "/api/vcenter/vm", nil)
	if err == nil || !strings.Contains(err.Error(), "freshly created session") {
		t.Fatalf("want terminal refusal, got %v", err)
	}
	if n := logins.Load(); n != 2 {
		t.Fatalf("logins = %d, want exactly 2 (initial + one retry)", n)
	}
	// Later requests on the same client do not log in again.
	for i := 0; i < 3; i++ {
		if _, err := c.Get(context.Background(), "/api/vcenter/vm", nil); err == nil {
			t.Fatal("want refusal")
		}
	}
	if n := logins.Load(); n != 2 {
		t.Fatalf("logins after refusal = %d, want still 2", n)
	}
}

func TestExplicitSessionHeaderRefusedOverRemoteHTTP(t *testing.T) {
	testenv.Isolate(t)
	c := newTestClient(t, "http://vcenter.example.com", "s3cret")
	req, _ := http.NewRequest(http.MethodGet, "http://vcenter.example.com/api/vcenter/vm", nil)
	req.Header.Set(SessionHeader, "abc")
	resp, err := c.HTTPClient.Transport.RoundTrip(req)
	if err != nil || resp.StatusCode != http.StatusMisdirectedRequest {
		t.Fatalf("want 421 refusal without dialing, got %v %v", resp, err)
	}
}

func TestRefusedLoginIsNotRetriedByParallelReads(t *testing.T) {
	testenv.Isolate(t)
	f := newFakeVCenter(t)
	t.Setenv(TLSFingerprintEnv, pinFor(f))
	var attempts atomic.Int32
	// Count login attempts with the wrong password through a wrapping server.
	mux := http.NewServeMux()
	mux.HandleFunc("/api/session", func(w http.ResponseWriter, r *http.Request) {
		attempts.Add(1)
		w.WriteHeader(http.StatusUnauthorized)
	})
	srv := httptest.NewTLSServer(mux)
	defer srv.Close()
	t.Setenv(TLSFingerprintEnv, CertFingerprint(srv.Certificate()))
	c := newTestClient(t, srv.URL, "wrong")
	done := make(chan struct{})
	for i := 0; i < 8; i++ {
		go func() {
			_, _ = c.Get(context.Background(), "/api/vcenter/vm", nil)
			done <- struct{}{}
		}()
	}
	for i := 0; i < 8; i++ {
		<-done
	}
	if n := attempts.Load(); n != 1 {
		t.Fatalf("login attempts with a refused password = %d, want 1", n)
	}
}

func TestNonBasicAuthorizationNeverLeavesOrigin(t *testing.T) {
	testenv.Isolate(t)
	var leaked atomic.Bool
	other := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "" {
			leaked.Store(true)
		}
	}))
	defer other.Close()
	c := newTestClient(t, "https://vcenter.example.com", "s3cret")
	req, _ := http.NewRequest(http.MethodGet, other.URL+"/x", nil)
	req.Header.Set("Authorization", "Bearer abc")
	if resp, err := c.HTTPClient.Transport.RoundTrip(req); err == nil {
		_ = resp.Body.Close()
	}
	if leaked.Load() {
		t.Fatal("non-Basic Authorization sent off-origin")
	}
	c2 := newTestClient(t, "http://vcenter.example.com", "s3cret")
	req2, _ := http.NewRequest(http.MethodGet, "http://vcenter.example.com/api/vcenter/vm", nil)
	req2.Header.Set("Authorization", "Bearer abc")
	resp, err := c2.HTTPClient.Transport.RoundTrip(req2)
	if err != nil || resp.StatusCode != http.StatusMisdirectedRequest {
		t.Fatalf("plain-http Authorization must be refused: %v %v", resp, err)
	}
}
