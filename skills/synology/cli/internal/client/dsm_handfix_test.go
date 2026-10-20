// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.
// HAND-AUTHORED. Exercises the HAND-FIX(dsm-login) and HAND-FIX(dsm-envelope)
// edits in client.go and session.go against a fake DSM.

package client

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"synology-pp-cli/internal/cliutil"
	"synology-pp-cli/internal/cliutil/testenv"
	"synology-pp-cli/internal/config"
	"synology-pp-cli/internal/dsm"
)

func dsmFake(t *testing.T) (*httptest.Server, *int32, *int32) {
	t.Helper()
	var logins, expireNext int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			_ = r.ParseForm()
			if r.PostForm.Get("passwd") != "pw" {
				_, _ = w.Write([]byte(`{"success":false,"error":{"code":400}}`))
				return
			}
			atomic.AddInt32(&logins, 1)
			_, _ = w.Write([]byte(`{"success":true,"data":{"sid":"s1","synotoken":"tok"}}`))
			return
		}
		if r.URL.Query().Get("_sid") != "s1" || r.Header.Get("X-SYNO-TOKEN") != "tok" {
			_, _ = w.Write([]byte(`{"success":false,"error":{"code":119}}`))
			return
		}
		if atomic.CompareAndSwapInt32(&expireNext, 1, 0) {
			_, _ = w.Write([]byte(`{"success":false,"error":{"code":119}}`))
			return
		}
		switch r.URL.Query().Get("api") {
		case "SYNO.ActiveBackup.Task":
			_, _ = w.Write([]byte(`{"success":true,"data":{"tasks":[{"task_id":5}],"total":1}}`))
		default:
			_, _ = w.Write([]byte(`{"success":false,"error":{"code":102}}`))
		}
	}))
	t.Cleanup(srv.Close)
	return srv, &logins, &expireNext
}

func TestDSMHandfixLoginTokenAndUnwrap(t *testing.T) {
	testenv.Isolate(t)
	srv, logins, expireNext := dsmFake(t)
	c := New(&config.Config{BaseURL: srv.URL, SynologyAccount: "admin", SynologyPassword: "pw"}, 5*time.Second, 0)
	c.NoCache = true
	path := "/webapi/entry.cgi?api=SYNO.ActiveBackup.Task&method=list&version=1"

	data, err := c.Get(context.Background(), path, nil)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != `{"tasks":[{"task_id":5}],"total":1}` {
		t.Fatalf("data not unwrapped: %s", data)
	}

	// An expired session (code 119) triggers exactly one relogin and a retry.
	atomic.StoreInt32(expireNext, 1)
	if _, err := c.Get(context.Background(), path, nil); err != nil {
		t.Fatalf("relogin retry failed: %v", err)
	}
	if got := atomic.LoadInt32(logins); got != 2 {
		t.Fatalf("logins = %d, want 2", got)
	}
}

func TestDSMHandfixFailureEnvelopeIsAnError(t *testing.T) {
	testenv.Isolate(t)
	srv, _, _ := dsmFake(t)
	c := New(&config.Config{BaseURL: srv.URL, SynologyAccount: "admin", SynologyPassword: "pw"}, 5*time.Second, 0)
	c.NoCache = true
	_, err := c.Get(context.Background(), "/webapi/entry.cgi?api=SYNO.Backup.Task&method=list&version=1", nil)
	if err == nil || !strings.Contains(err.Error(), "DSM error 102") {
		t.Fatalf("err = %v, want DSM error 102", err)
	}
	apiErr, ok := err.(*APIError)
	if !ok || apiErr.StatusCode != http.StatusNotFound {
		t.Fatalf("err type/status = %T %v", err, err)
	}
}

func TestDSMHandfixNoCredentialsNamesEnvVars(t *testing.T) {
	testenv.Isolate(t)
	srv, logins, _ := dsmFake(t)
	c := New(&config.Config{BaseURL: srv.URL}, 5*time.Second, 0)
	c.NoCache = true
	_, err := c.Get(context.Background(), "/webapi/entry.cgi?api=SYNO.ActiveBackup.Task&method=list&version=1", nil)
	if err == nil || !strings.Contains(err.Error(), "SYNOLOGY_ACCOUNT") {
		t.Fatalf("err = %v, want a hint naming SYNOLOGY_ACCOUNT", err)
	}
	if *logins != 0 {
		t.Fatalf("logins = %d, want 0 without credentials", *logins)
	}
}

func TestDSMHandfixLoginRefusesCrossOriginRedirect(t *testing.T) {
	testenv.Isolate(t)
	var leaked bool
	evil := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		leaked = leaked || r.PostForm.Get("passwd") != "" || r.Header.Get("X-SYNO-TOKEN") != ""
		_, _ = w.Write([]byte(`{"success":true,"data":{"sid":"evil","synotoken":"evil"}}`))
	}))
	defer evil.Close()
	nas := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, evil.URL+r.URL.RequestURI(), http.StatusTemporaryRedirect)
	}))
	defer nas.Close()
	c := New(&config.Config{BaseURL: nas.URL, SynologyAccount: "admin", SynologyPassword: "hunter2"}, 5*time.Second, 0)
	c.NoCache = true
	if _, err := c.Get(context.Background(), "/webapi/entry.cgi?api=SYNO.ActiveBackup.Task&method=list&version=1", nil); err == nil {
		t.Fatal("login followed a cross-origin redirect")
	}
	if leaked {
		t.Fatal("password or SynoToken reached another origin")
	}
}

func TestDSMHandfixMalformedPinRefusesTLS(t *testing.T) {
	testenv.Isolate(t)
	t.Setenv("SYNOLOGY_TLS_FINGERPRINT", "not-a-fingerprint")
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"success":true,"data":{"sid":"s","synotoken":"t"}}`))
	}))
	defer srv.Close()
	hc := newHTTPClient(5*time.Second, nil)
	resp, err := hc.Get(srv.URL)
	if err == nil {
		resp.Body.Close()
	}
	// The refusal must come from the pin, not from ordinary verification of the
	// test certificate (which a publicly trusted certificate would pass).
	if err == nil || !strings.Contains(err.Error(), "TLS refused") {
		t.Fatalf("err = %v, want the malformed pin to refuse TLS", err)
	}
}

func TestDSMHandfixDataRedirectIsRefused(t *testing.T) {
	testenv.Isolate(t)
	var sawSID bool
	evil := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sawSID = strings.Contains(r.Header.Get("Referer"), "s1") || r.URL.Query().Get("_sid") != "" || r.Header.Get("X-SYNO-TOKEN") != ""
		_, _ = w.Write([]byte(`{"success":true,"data":{"tasks":[]}}`))
	}))
	defer evil.Close()
	nas := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			_, _ = w.Write([]byte(`{"success":true,"data":{"sid":"s1","synotoken":"tok"}}`))
			return
		}
		http.Redirect(w, r, evil.URL+r.URL.RequestURI(), http.StatusFound)
	}))
	defer nas.Close()
	c := New(&config.Config{BaseURL: nas.URL, SynologyAccount: "admin", SynologyPassword: "pw"}, 5*time.Second, 0)
	c.NoCache = true
	_, err := c.Get(context.Background(), "/webapi/entry.cgi?api=SYNO.ActiveBackup.Task&method=list&version=1", nil)
	if err == nil {
		t.Fatal("a cross-origin answer was accepted as the NAS's data")
	}
	if sawSID {
		t.Fatal("the sid or SynoToken reached another origin")
	}
}

func TestDSMHandfixInvalidateSIDOnlyClearsCurrent(t *testing.T) {
	m := newSessionManager(time.Second, "")
	m.token, m.synoToken = "new", "tok"
	m.InvalidateSID("old")
	if m.Token() != "new" {
		t.Fatal("a late rejection of an old sid cleared the new session")
	}
	m.InvalidateSID("new")
	if sid, st := m.Snapshot(); sid != "" || st != "" {
		t.Fatal("rejected current sid was kept")
	}
}

func TestDSMHandfixSessionFileKeepsFleetDeviceTokens(t *testing.T) {
	testenv.Isolate(t)
	srv, _, _ := dsmFake(t)
	dir, err := cliutil.DataDir()
	if err != nil {
		t.Fatal(err)
	}
	if err := dsm.SaveDeviceID(dir, srv.URL, "fleet-acct", "did-fleet"); err != nil {
		t.Fatal(err)
	}
	c := New(&config.Config{BaseURL: srv.URL, SynologyAccount: "admin", SynologyPassword: "pw"}, 5*time.Second, 0)
	if _, err := c.Session.EnsureToken(); err != nil {
		t.Fatal(err)
	}
	if err := c.Session.SaveError(); err != nil {
		t.Fatal(err)
	}
	if got := dsm.CachedDeviceID(dir, srv.URL, "fleet-acct"); got != "did-fleet" {
		t.Fatalf("generated login dropped the fleet account's device token (got %q)", got)
	}
	m := newSessionManager(time.Second, "")
	m.ConfigureDSM(srv.URL, dsm.Credentials{Account: "fleet-acct", Password: "x"})
	if m.deviceID != "did-fleet" {
		t.Fatalf("session manager did not load the per-account token (got %q)", m.deviceID)
	}
}

func TestDSMHandfixPinOverPlainHTTPRefusesLogin(t *testing.T) {
	testenv.Isolate(t)
	t.Setenv("SYNOLOGY_TLS_FINGERPRINT", strings.Repeat("ab", 32))
	srv, logins, _ := dsmFake(t)
	c := New(&config.Config{BaseURL: srv.URL, SynologyAccount: "admin", SynologyPassword: "pw"}, 5*time.Second, 0)
	c.NoCache = true
	_, err := c.Get(context.Background(), "/webapi/entry.cgi?api=SYNO.ActiveBackup.Task&method=list&version=1", nil)
	if err == nil || !strings.Contains(err.Error(), "plain HTTP") || atomic.LoadInt32(logins) != 0 {
		t.Fatalf("err = %v, logins = %d: a pin over http:// must refuse to send the password", err, *logins)
	}
}

func TestDSMHandfixSessionIsBoundToOrigin(t *testing.T) {
	testenv.Isolate(t)
	srv, _, _ := dsmFake(t) // http://127.0.0.1:port
	c := New(&config.Config{BaseURL: srv.URL, SynologyAccount: "admin", SynologyPassword: "pw"}, 5*time.Second, 0)
	if _, err := c.Session.EnsureToken(); err != nil {
		t.Fatal(err)
	}
	// Same host, other scheme: the cached sid must not be reused.
	other := "https://" + strings.TrimPrefix(srv.URL, "http://")
	m := newSessionManager(time.Second, "")
	m.ConfigureDSM(other, dsm.Credentials{Account: "admin", Password: "pw"})
	if sid, _ := m.Snapshot(); sid != "" {
		t.Fatalf("a sid cached for %s was loaded for %s", srv.URL, other)
	}
	// A pin over plain HTTP refuses even a cached session.
	t.Setenv("SYNOLOGY_TLS_FINGERPRINT", strings.Repeat("ab", 32))
	m2 := newSessionManager(time.Second, "")
	m2.ConfigureDSM(srv.URL, dsm.Credentials{Account: "admin", Password: "pw"})
	if tok, err := m2.EnsureToken(); err == nil || tok != "" {
		t.Fatalf("pin over http reused the cached sid: tok=%q err=%v", tok, err)
	}
}

func TestDSMHandfixUserinfoBaseURLNeverLeaks(t *testing.T) {
	testenv.Isolate(t)
	c := New(&config.Config{BaseURL: "https://admin:pa%ss@nas.invalid:5001", SynologyAccount: "admin", SynologyPassword: "pw"}, time.Second, 0)
	_, err := c.Session.EnsureToken()
	if err == nil || strings.Contains(err.Error(), "pa%ss") {
		t.Fatalf("err = %v", err)
	}
}

func TestDSMHandfixPlaceholderAddressIsRefusedLiveButPreviews(t *testing.T) {
	testenv.Isolate(t)
	c := New(&config.Config{BaseURL: "https://nas.example.com:5001", SynologyAccount: "admin", SynologyPassword: "pw"}, time.Second, 0)
	c.NoCache = true
	if _, err := c.Session.EnsureToken(); err == nil || !strings.Contains(err.Error(), "placeholder") {
		t.Fatalf("login to the placeholder host: err = %v", err)
	}
	c.DryRun = true
	if _, err := c.Get(context.Background(), "/webapi/entry.cgi?api=SYNO.ActiveBackup.Task&method=list&version=1", nil); err != nil {
		t.Fatalf("dry run with the placeholder address failed: %v", err)
	}
}

func TestDSMHandfixEnsureSnapshotFailsInsteadOfStaleSID(t *testing.T) {
	testenv.Isolate(t)
	srv, _, _ := dsmFake(t)
	c := New(&config.Config{BaseURL: srv.URL, SynologyAccount: "admin", SynologyPassword: "pw"}, 5*time.Second, 0)
	sid, st, err := c.Session.EnsureSnapshot()
	if err != nil || sid == "" || st == "" {
		t.Fatalf("snapshot = %q %q %v", sid, st, err)
	}
	// No credentials: nothing to log in with, and no error.
	m := newSessionManager(time.Second, "")
	if sid, _, err := m.EnsureSnapshot(); sid != "" || err != nil {
		t.Fatalf("credential-less snapshot = %q %v", sid, err)
	}
}

func TestDSMHandfixHTMLDiagnosticMasksSID(t *testing.T) {
	testenv.Isolate(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			_, _ = w.Write([]byte(`{"success":true,"data":{"sid":"SECRETSID","synotoken":"tok"}}`))
			return
		}
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte(`<html><head><title>Rejected ` + r.URL.RawQuery + `</title></head></html>`))
	}))
	defer srv.Close()
	c := New(&config.Config{BaseURL: srv.URL, SynologyAccount: "admin", SynologyPassword: "pw"}, 5*time.Second, 0)
	c.NoCache = true
	_, err := c.Get(context.Background(), "/webapi/entry.cgi?api=SYNO.ActiveBackup.Task&method=list&version=1", nil)
	if err == nil || strings.Contains(err.Error(), "SECRETSID") {
		t.Fatalf("err = %v", err)
	}
}

func TestDSMHandfixReadWithoutDataIsAnError(t *testing.T) {
	testenv.Isolate(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			_, _ = w.Write([]byte(`{"success":true,"data":{"sid":"s1","synotoken":"tok"}}`))
			return
		}
		_, _ = w.Write([]byte(r.URL.Query().Get("reply")))
	}))
	defer srv.Close()
	c := New(&config.Config{BaseURL: srv.URL, SynologyAccount: "admin", SynologyPassword: "pw"}, 5*time.Second, 0)
	c.NoCache = true
	for _, reply := range []string{`{"success":true}`, `[]`, `null`, `{"tasks":[]}`} {
		if _, err := c.Get(context.Background(), "/webapi/entry.cgi?api=SYNO.ActiveBackup.Task&method=list&version=1&reply="+url.QueryEscape(reply), nil); err == nil {
			t.Fatalf("a read answered with %s was accepted", reply)
		}
	}
	if _, err := c.GetMutating(context.Background(), "/webapi/entry.cgi?api=SYNO.ActiveBackup.Task&method=backup&version=1&reply="+url.QueryEscape(`{"success":true}`), nil); err != nil {
		t.Fatalf("a write acknowledged without data failed: %v", err)
	}
}

func TestDSMHandfixBinaryContentTypeStillValidated(t *testing.T) {
	testenv.Isolate(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			_, _ = w.Write([]byte(`{"success":true,"data":{"sid":"s1","synotoken":"tok"}}`))
			return
		}
		w.Header().Set("Content-Type", "application/octet-stream")
		_, _ = w.Write([]byte(r.URL.Query().Get("reply")))
	}))
	defer srv.Close()
	c := New(&config.Config{BaseURL: srv.URL, SynologyAccount: "admin", SynologyPassword: "pw"}, 5*time.Second, 0)
	c.NoCache = true
	for _, reply := range []string{`null`, `{"success":false,"error":{"code":102}}`} {
		if _, err := c.Get(context.Background(), "/webapi/entry.cgi?api=SYNO.ActiveBackup.Task&method=list&version=1&reply="+url.QueryEscape(reply), nil); err == nil {
			t.Fatalf("octet-stream %s accepted as data", reply)
		}
	}
}

func TestDSMHandfixWriteNeedsSuccessEnvelope(t *testing.T) {
	testenv.Isolate(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			_, _ = w.Write([]byte(`{"success":true,"data":{"sid":"s1","synotoken":"tok"}}`))
			return
		}
		_, _ = w.Write([]byte(r.URL.Query().Get("reply")))
	}))
	defer srv.Close()
	c := New(&config.Config{BaseURL: srv.URL, SynologyAccount: "admin", SynologyPassword: "pw"}, 5*time.Second, 0)
	c.NoCache = true
	for _, reply := range []string{`null`, `[]`, `{"started":true}`} {
		if _, err := c.GetMutating(context.Background(), "/webapi/entry.cgi?api=SYNO.ActiveBackup.Task&method=backup&version=1&reply="+url.QueryEscape(reply), nil); err == nil {
			t.Fatalf("a write answered with %s read as success", reply)
		}
	}
}

func TestDSMHandfixEmptyAccountNeverReusesAnotherSession(t *testing.T) {
	testenv.Isolate(t)
	srv, _, _ := dsmFake(t)
	c := New(&config.Config{BaseURL: srv.URL, SynologyAccount: "admin", SynologyPassword: "pw"}, 5*time.Second, 0)
	if _, err := c.Session.EnsureToken(); err != nil {
		t.Fatal(err)
	}
	m := newSessionManager(time.Second, "")
	m.ConfigureDSM(srv.URL, dsm.Credentials{})
	if sid, _ := m.Snapshot(); sid != "" {
		t.Fatal("a configuration without an account reused admin's session")
	}
}

func TestDSMHandfixDiagnosticsMaskSynoToken(t *testing.T) {
	testenv.Isolate(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			_, _ = w.Write([]byte(`{"success":true,"data":{"sid":"SIDVALUE1","synotoken":"TOKENVALUE1"}}`))
			return
		}
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte(`<html><head><title>Rejected ` + r.Header.Get("X-SYNO-TOKEN") + `</title></head></html>`))
	}))
	defer srv.Close()
	c := New(&config.Config{BaseURL: srv.URL, SynologyAccount: "admin", SynologyPassword: "pw"}, 5*time.Second, 0)
	c.NoCache = true
	_, err := c.Get(context.Background(), "/webapi/entry.cgi?api=SYNO.ActiveBackup.Task&method=list&version=1", nil)
	if err == nil || strings.Contains(err.Error(), "TOKENVALUE1") {
		t.Fatalf("err = %v", err)
	}
}

func TestDSMHandfixMasksBeforeTruncatingAndJSONEscapes(t *testing.T) {
	testenv.Isolate(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			_, _ = w.Write([]byte(`{"success":true,"data":{"sid":"SIDVALUE2","synotoken":"TOKENVALUE2LONGSECRET"}}`))
			return
		}
		if r.URL.Query().Get("mode") == "html" {
			w.Header().Set("Content-Type", "text/html")
			_, _ = w.Write([]byte(`<html><head><title>` + strings.Repeat("x", 150) + r.Header.Get("X-SYNO-TOKEN") + `</title></head></html>`))
			return
		}
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":"bad password quote\"secretpw"}`))
	}))
	defer srv.Close()
	c := New(&config.Config{BaseURL: srv.URL, SynologyAccount: "admin", SynologyPassword: `quote"secretpw`}, 5*time.Second, 0)
	c.NoCache = true
	_, err := c.Get(context.Background(), "/webapi/entry.cgi?api=SYNO.ActiveBackup.Task&method=list&version=1&mode=html", nil)
	if err == nil || strings.Contains(err.Error(), "TOKENVAL") {
		t.Fatalf("truncated title leaked the token prefix: %v", err)
	}
	_, err = c.Get(context.Background(), "/webapi/entry.cgi?api=SYNO.ActiveBackup.Task&method=list&version=1&mode=json", nil)
	if err == nil || strings.Contains(err.Error(), "secretpw") {
		t.Fatalf("JSON-escaped password leaked: %v", err)
	}
}

func TestDSMHandfixMasksHTMLEntityAndDoubleEncodedSecrets(t *testing.T) {
	testenv.Isolate(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			_, _ = w.Write([]byte(`{"success":true,"data":{"sid":"SIDVALUE3","synotoken":"TOK3"}}`))
			return
		}
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte(`<html><head><title>pw alpha&amp;OMEGASECRET and alpha%2526OMEGASECRET</title></head></html>`))
	}))
	defer srv.Close()
	c := New(&config.Config{BaseURL: srv.URL, SynologyAccount: "admin", SynologyPassword: "alpha&OMEGASECRET"}, 5*time.Second, 0)
	c.NoCache = true
	_, err := c.Get(context.Background(), "/webapi/entry.cgi?api=SYNO.ActiveBackup.Task&method=list&version=1", nil)
	if err == nil || strings.Contains(err.Error(), "OMEGA") {
		t.Fatalf("an HTML-escaped or doubly encoded password leaked: %v", err)
	}
}

func TestDSMHandfixMasksNestedEntitiesAndLowercaseEscapes(t *testing.T) {
	testenv.Isolate(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			_, _ = w.Write([]byte(`{"success":true,"data":{"sid":"SIDVALUE4","synotoken":"TOK4"}}`))
			return
		}
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte(`<html><head><title>` + r.URL.Query().Get("t") + `</title></head></html>`))
	}))
	defer srv.Close()
	for pw, title := range map[string]string{"alpha&OMEGASECRET": "alpha&#38;#38;OMEGASECRET", "alpha/OMEGASECRET": "alpha%2fOMEGASECRET"} {
		c := New(&config.Config{BaseURL: srv.URL, SynologyAccount: "admin", SynologyPassword: pw}, 5*time.Second, 0)
		c.NoCache = true
		_, err := c.Get(context.Background(), "/webapi/entry.cgi?api=SYNO.ActiveBackup.Task&method=list&version=1&t="+url.QueryEscape(title), nil)
		if err == nil || strings.Contains(err.Error(), "OMEGA") {
			t.Fatalf("password %q leaked: %v", pw, err)
		}
	}
}

func TestDSMHandfixHTMLAndProxyBodiesAreNotShown(t *testing.T) {
	testenv.Isolate(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			_, _ = w.Write([]byte(`{"success":true,"data":{"sid":"SIDVALUE5","synotoken":"TOK5"}}`))
			return
		}
		if r.URL.Query().Get("mode") == "502" {
			w.WriteHeader(http.StatusBadGateway)
		}
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte("<html><head><title>alpha​OMEGASECRET</title></head></html>"))
	}))
	defer srv.Close()
	c := New(&config.Config{BaseURL: srv.URL, SynologyAccount: "admin", SynologyPassword: "alphaOMEGASECRET"}, 5*time.Second, 0)
	c.NoCache = true
	for _, mode := range []string{"200", "502"} {
		_, err := c.Get(context.Background(), "/webapi/entry.cgi?api=SYNO.ActiveBackup.Task&method=list&version=1&mode="+mode, nil)
		if err == nil || strings.Contains(err.Error(), "OMEGA") {
			t.Fatalf("mode %s: page text shown: %v", mode, err)
		}
	}
}

func TestDSMHandfixErrorCodesAreMasked(t *testing.T) {
	testenv.Isolate(t)
	const pw = "987654321035"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			if r.URL.Query().Get("fail") == "1" || r.Header.Get("X-Fail") == "1" {
				_, _ = w.Write([]byte(`{"success":false,"error":{"code":` + pw + `}}`))
				return
			}
			_, _ = w.Write([]byte(`{"success":true,"data":{"sid":"SIDVALUE6","synotoken":"TOK6"}}`))
			return
		}
		_, _ = w.Write([]byte(`{"success":false,"error":{"code":` + pw + `}}`))
	}))
	defer srv.Close()
	c := New(&config.Config{BaseURL: srv.URL, SynologyAccount: "admin", SynologyPassword: pw}, 5*time.Second, 0)
	c.NoCache = true
	_, err := c.Get(context.Background(), "/webapi/entry.cgi?api=SYNO.ActiveBackup.Task&method=list&version=1", nil)
	if err == nil || strings.Contains(err.Error(), pw) {
		t.Fatalf("an envelope failure echoed the password: %v", err)
	}
	// A login failure whose code echoes the password.
	bad := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"success":false,"error":{"code":` + pw + `}}`))
	}))
	defer bad.Close()
	m := newSessionManager(time.Second, "")
	m.ConfigureDSM(bad.URL, dsm.Credentials{Account: "admin", Password: pw})
	if _, err := m.EnsureToken(); err == nil || strings.Contains(err.Error(), pw) {
		t.Fatalf("a login failure echoed the password: %v", err)
	}
}

func TestDSMHandfixNoResponseHeaderTextInDiagnostics(t *testing.T) {
	testenv.Isolate(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			_, _ = w.Write([]byte(`{"success":true,"data":{"sid":"SIDVALUE38","synotoken":"TOK38"}}`))
			return
		}
		w.Header().Set("cf-mitigated", "challenge HDRSECRET38")
		w.Header().Set("X-Request-ID", "SIDVALUE38")
		w.Header().Set("Retry-After", "0")
		if r.URL.Query().Get("mode") == "403" {
			w.WriteHeader(http.StatusForbidden)
		} else {
			w.WriteHeader(http.StatusTooManyRequests)
		}
	}))
	defer srv.Close()
	c := New(&config.Config{BaseURL: srv.URL, SynologyAccount: "admin", SynologyPassword: "pw"}, 5*time.Second, 0)
	c.NoCache = true
	for _, mode := range []string{"403", "429"} {
		_, err := c.Get(context.Background(), "/webapi/entry.cgi?api=SYNO.ActiveBackup.Task&method=list&version=1&mode="+mode, nil)
		if err == nil || strings.Contains(err.Error(), "HDRSECRET38") || strings.Contains(err.Error(), "SIDVALUE38") {
			t.Fatalf("mode %s: header text in diagnostics: %v", mode, err)
		}
	}
}
