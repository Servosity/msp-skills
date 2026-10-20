// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// HAND-AUTHORED. Pins HAND-FIX(dsm-probe-before-session) and
// HAND-FIX(dsm-session-origin-bound).

package client

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"synology-pp-cli/internal/cliutil"
	"synology-pp-cli/internal/cliutil/testenv"
	"synology-pp-cli/internal/config"
	"synology-pp-cli/internal/dsm"
	"synology-pp-cli/internal/dsm/dsmtest"
)

// wire records what every request carried: method, URL, Cookie, X-SYNO-TOKEN.
type wire struct {
	mu  sync.Mutex
	log []string
}

func (w *wire) add(r *http.Request) {
	b, _ := io.ReadAll(r.Body)
	w.mu.Lock()
	defer w.mu.Unlock()
	w.log = append(w.log, r.Method+" "+r.URL.String()+" cookie="+r.Header.Get("Cookie")+" token="+r.Header.Get("X-SYNO-TOKEN")+" body="+string(b))
}

func (w *wire) all() string {
	w.mu.Lock()
	defer w.mu.Unlock()
	return strings.Join(w.log, "\n")
}

func (w *wire) count(sub string) int {
	w.mu.Lock()
	defer w.mu.Unlock()
	n := 0
	for _, l := range w.log {
		if strings.Contains(l, sub) {
			n++
		}
	}
	return n
}

// sentinel is the raw session id a config's auth_header carries.
const sentinel = "SID-sentinel-9917"

// routerHost is a host that is not DSM (a router's admin page on the NAS's
// address), recording every request.
func routerHost(t *testing.T) (*httptest.Server, *wire) {
	t.Helper()
	w := &wire{}
	srv := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		w.add(r)
		rw.Header().Set("Content-Type", "text/html")
		_, _ = io.WriteString(rw, "<html><body>router login</body></html>")
	}))
	t.Cleanup(srv.Close)
	return srv, w
}

// dsmHost is a DSM stand-in: it answers the probe and a login (sid
// SID-FRESH-1, SynoToken TOK-FRESH-1) and every read, recording every request.
func dsmHost(t *testing.T) (*httptest.Server, *wire) {
	t.Helper()
	w := &wire{}
	srv := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		w.add(r)
		if dsmtest.ServeAPIInfo(rw, r) {
			return
		}
		if r.Method == http.MethodPost {
			_, _ = io.WriteString(rw, `{"success":true,"data":{"sid":"SID-FRESH-1","synotoken":"TOK-FRESH-1"}}`)
			return
		}
		_, _ = io.WriteString(rw, `{"success":true,"data":{"tasks":[]}}`)
	}))
	t.Cleanup(srv.Close)
	return srv, w
}

// writeSessionFile writes a session cache record at path.
func writeSessionFile(t *testing.T, path string, rec map[string]any) {
	t.Helper()
	b, _ := json.Marshal(rec)
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, b, 0o600); err != nil {
		t.Fatal(err)
	}
}

// TestRawSessionGoesOnlyToADSM: no request carrying a session or a
// credential reaches an address that has not answered SYNO.API.Info as DSM
// in this process. A raw sid from the config's auth_header (with no account
// or password, so no login runs) and a cached session restored from disk
// used to go straight to whatever answered the NAS address; now a router
// page gets the unauthenticated probe and nothing else, the error says the
// session was not sent, and a real DSM gets the sid after one probe per
// address per process.
func TestRawSessionGoesOnlyToADSM(t *testing.T) {
	t.Run("auth_header sid", func(t *testing.T) {
		testenv.Isolate(t)
		dsm.ForgetVerifiedAddresses()
		router, seen := routerHost(t)
		c := New(&config.Config{BaseURL: router.URL, AuthHeaderVal: sentinel}, 5*time.Second, 0)
		c.NoCache = true
		_, err := c.Get(context.Background(), tasksPath, nil)
		var notDSM *dsm.NotDSMError
		if !errors.As(err, &notDSM) || !strings.Contains(err.Error(), "the session was not sent") {
			t.Fatalf("err = %v, want a refusal saying the session was not sent", err)
		}
		if strings.Contains(seen.all(), "SID-sentinel-9917") || seen.count("SYNO.API.Info") != 1 || seen.count("") != 1 {
			t.Fatalf("a host that is not DSM received more than the probe:\n%s", seen.all())
		}
	})
	t.Run("cached session and cookie", func(t *testing.T) {
		testenv.Isolate(t)
		dsm.ForgetVerifiedAddresses()
		router, seen := routerHost(t)
		dir, err := cliutil.DataDir()
		if err != nil {
			t.Fatal(err)
		}
		writeSessionFile(t, filepath.Join(dir, dsm.SessionFileName(router.URL)), map[string]any{
			"token": "SID-CACHED-77", "synotoken": "TOK-CACHED-77", "account": "admin", "origin": router.URL,
			"cookies": []map[string]any{{"url": router.URL + "/", "name": "id", "value": "COOKIE-CACHED-77", "path": "/"}},
			"saved":   time.Now(),
		})
		c := New(&config.Config{BaseURL: router.URL, SynologyAccount: "admin"}, 5*time.Second, 0)
		c.NoCache = true
		if sid, _ := c.Session.Snapshot(); sid != "SID-CACHED-77" {
			t.Fatalf("fixture: the cached session for this exact origin and account was not restored (sid %q)", sid)
		}
		_, err = c.Get(context.Background(), tasksPath, nil)
		var notDSM *dsm.NotDSMError
		if !errors.As(err, &notDSM) || !strings.Contains(err.Error(), "the session was not sent") {
			t.Fatalf("err = %v, want a refusal saying the session was not sent", err)
		}
		if strings.Contains(seen.all(), "CACHED-77") || seen.count("") != 1 {
			t.Fatalf("a host that is not DSM received the cached session or more than the probe:\n%s", seen.all())
		}
	})
	t.Run("a DSM gets the sid after one probe per address per process", func(t *testing.T) {
		testenv.Isolate(t)
		dsm.ForgetVerifiedAddresses()
		nas, seen := dsmHost(t)
		for i := 0; i < 2; i++ {
			c := New(&config.Config{BaseURL: nas.URL, AuthHeaderVal: sentinel}, 5*time.Second, 0)
			c.NoCache = true
			for j := 0; j < 2; j++ {
				if _, err := c.Get(context.Background(), tasksPath, nil); err != nil {
					t.Fatal(err)
				}
			}
		}
		if n := seen.count("_sid=SID-sentinel-9917"); n != 4 {
			t.Fatalf("the DSM received the sid %d times, want 4:\n%s", n, seen.all())
		}
		if n := seen.count("SYNO.API.Info"); n != 1 {
			t.Fatalf("the address was probed %d times, want once per process:\n%s", n, seen.all())
		}
	})
	t.Run("a login and the requests after it share the one probe", func(t *testing.T) {
		testenv.Isolate(t)
		dsm.ForgetVerifiedAddresses()
		nas, seen := dsmHost(t)
		c := New(&config.Config{BaseURL: nas.URL, SynologyAccount: "admin", SynologyPassword: "pw-probe-once"}, 5*time.Second, 0)
		c.NoCache = true
		for j := 0; j < 2; j++ {
			if _, err := c.Get(context.Background(), tasksPath, nil); err != nil {
				t.Fatal(err)
			}
		}
		if seen.count("POST ") != 1 || seen.count("_sid=SID-FRESH-1") != 2 || seen.count("SYNO.API.Info") != 1 {
			t.Fatalf("want one probe, one login, two reads:\n%s", seen.all())
		}
	})
}

// TestLegacySessionNeverLoaded: an unexpired origin-less session.json (the
// legacy data-dir and config-dir files, no account, cookies for the NAS
// host) and a per-origin file without an account were loaded into the
// session manager before the origin was known; ConfigureDSM then cleared
// the sid but kept the cookie jar, so a request to http://<same host> carried
// the cookies (and an account-less record's trusted-device token went into
// the next login). A session is now restored only for the exact origin and
// account it was saved for: no sid, cookie or device token from such a file
// is ever sent.
func TestLegacySessionNeverLoaded(t *testing.T) {
	for _, withLogin := range []bool{true, false} {
		testenv.Isolate(t)
		dsm.ForgetVerifiedAddresses()
		nas, seen := dsmHost(t) // http://127.0.0.1:<port>
		dataDir, err := cliutil.DataDir()
		if err != nil {
			t.Fatal(err)
		}
		configDir, err := cliutil.ConfigDir()
		if err != nil {
			t.Fatal(err)
		}
		hostPort := strings.TrimPrefix(nas.URL, "http://")
		legacy := map[string]any{
			"token": "SID-LEGACY-1", "synotoken": "TOK-LEGACY-1", "device_id": "DID-LEGACY-1",
			"cookies": []map[string]any{
				{"url": "https://" + hostPort + "/", "name": "id", "value": "COOKIE-LEGACY-HTTPS", "path": "/"},
				{"url": nas.URL + "/", "name": "did", "value": "COOKIE-LEGACY-HTTP", "path": "/"},
			},
			"saved": time.Now(),
		}
		writeSessionFile(t, filepath.Join(dataDir, "session.json"), legacy)
		writeSessionFile(t, filepath.Join(configDir, "session.json"), legacy)
		perOrigin := map[string]any{}
		for k, v := range legacy {
			perOrigin[k] = v
		}
		perOrigin["origin"] = nas.URL // the right origin, but no account
		writeSessionFile(t, filepath.Join(dataDir, dsm.SessionFileName(nas.URL)), perOrigin)

		cfg := &config.Config{BaseURL: nas.URL, Path: filepath.Join(configDir, "config.toml")}
		if withLogin {
			cfg.SynologyAccount, cfg.SynologyPassword = "admin", "pw-legacy-1"
		}
		c := New(cfg, 5*time.Second, 0)
		c.NoCache = true
		if _, err := c.Get(context.Background(), tasksPath, nil); err != nil {
			t.Fatal(err)
		}
		if strings.Contains(seen.all(), "LEGACY") {
			t.Fatalf("login=%v: a legacy or account-less session was sent:\n%s", withLogin, seen.all())
		}
		if withLogin && seen.count("_sid=SID-FRESH-1") != 1 {
			t.Fatalf("the fresh login's sid was not used:\n%s", seen.all())
		}
	}
}

// TestReconfigureResetsTheSession: pointing the session manager at another
// origin drops the sid, the SynoToken, the device token and every cookie.
func TestReconfigureResetsTheSession(t *testing.T) {
	testenv.Isolate(t)
	m := newSessionManager(time.Second, "")
	m.ConfigureDSM("https://127.0.0.1:5001", dsm.Credentials{Account: "admin"})
	u := mustParse(t, "https://127.0.0.1:5001/webapi/entry.cgi")
	m.jar.SetCookies(u, []*http.Cookie{{Name: "id", Value: "COOKIE-OLD"}})
	m.mu.Lock()
	m.token, m.synoToken, m.deviceID = "SID-OLD", "TOK-OLD", "DID-OLD"
	m.mu.Unlock()
	if len(m.jar.Cookies(u)) != 1 {
		t.Fatal("fixture: the cookie was not stored for its own origin")
	}
	m.ConfigureDSM("http://127.0.0.1:5000", dsm.Credentials{Account: "admin"})
	for _, probe := range []string{"http://127.0.0.1:5000/", "https://127.0.0.1:5001/webapi/entry.cgi", "http://127.0.0.1:5001/"} {
		if got := m.jar.Cookies(mustParse(t, probe)); len(got) != 0 {
			t.Fatalf("a cookie survived reconfiguration for %s: %v", probe, got)
		}
	}
	if sid, st := m.Snapshot(); sid != "" || st != "" || m.deviceID != "" {
		t.Fatalf("session fields survived reconfiguration: sid=%q token=%q device=%q", sid, st, m.deviceID)
	}
}

func mustParse(t *testing.T, raw string) *url.URL {
	t.Helper()
	u, err := url.Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	return u
}
