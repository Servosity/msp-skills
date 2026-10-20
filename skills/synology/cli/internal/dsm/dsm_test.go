// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// HAND-AUTHORED.

package dsm

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"

	"synology-pp-cli/internal/dsm/dsmtest"
)

func TestEnvelope(t *testing.T) {
	cases := []struct {
		name       string
		body       string
		wantData   string
		wantCode   int
		wantFailed bool
	}{
		{"success with data", `{"success":true,"data":{"tasks":[1]}}`, `{"tasks":[1]}`, 0, false},
		{"failure", `{"success":false,"error":{"code":119}}`, "", 119, true},
		{"failure without code", `{"success":false}`, "", 100, true},
		{"success no data", `{"success":true}`, "", 0, false},
		{"null data", `{"success":true,"data":null}`, "", 0, false},
		{"not an envelope", `{"tasks":[]}`, "", 0, false},
		{"array", `[1,2]`, "", 0, false},
		{"binary", "\x89PNG", "", 0, false},
	}
	for _, tc := range cases {
		data, code, failed := Envelope([]byte(tc.body))
		if string(data) != tc.wantData || code != tc.wantCode || failed != tc.wantFailed {
			t.Errorf("%s: got (%q,%d,%v) want (%q,%d,%v)", tc.name, data, code, failed, tc.wantData, tc.wantCode, tc.wantFailed)
		}
	}
}

func TestErrorTextAndStatus(t *testing.T) {
	if got := ErrorText(403, "SYNO.API.Auth"); !strings.Contains(got, "one-time code") {
		t.Errorf("auth 403 text = %q", got)
	}
	// 4xx codes mean something else outside SYNO.API.Auth.
	if got := ErrorText(403, "SYNO.FileStation.List"); got != "DSM error 403" {
		t.Errorf("non-auth 403 text = %q", got)
	}
	if got := ErrorText(102, "SYNO.ActiveBackup.Task"); !strings.Contains(got, "not be installed") {
		t.Errorf("102 text = %q", got)
	}
	for code, want := range map[int]int{119: 401, 106: 401, 105: 403, 102: 404, 101: 400} {
		if got := HTTPStatus(code, "SYNO.ActiveBackup.Task"); got != want {
			t.Errorf("HTTPStatus(%d) = %d want %d", code, got, want)
		}
	}
	if got := HTTPStatus(400, "SYNO.API.Auth"); got != 401 {
		t.Errorf("auth 400 status = %d", got)
	}
}

func TestScrub(t *testing.T) {
	in := `Post "https://nas:5001/webapi/entry.cgi?account=a&passwd=testpass0&otp_code=123456&_sid=abc": dial`
	got := Scrub(in)
	for _, secret := range []string{"testpass0", "123456", "=abc"} {
		if strings.Contains(got, secret) {
			t.Fatalf("Scrub left %q in %q", secret, got)
		}
	}
	if !strings.Contains(got, "account=a") {
		t.Fatalf("Scrub removed a non-secret: %q", got)
	}
}

func TestAPIName(t *testing.T) {
	if got := APIName("/webapi/entry.cgi?api=SYNO.ActiveBackup.Task&method=list&version=1"); got != "SYNO.ActiveBackup.Task" {
		t.Fatalf("APIName = %q", got)
	}
}

// fakeNAS answers login with a fixed sid/SynoToken and serves one data call
// that requires both. expireFirst makes the first data call answer code 119.
func fakeNAS(t *testing.T, expireFirst bool) (*httptest.Server, *int32) {
	t.Helper()
	var logins int32
	var dataCalls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if dsmtest.ServeAPIInfo(w, r) {
			return
		}
		if r.URL.Path != "/webapi/entry.cgi" {
			http.NotFound(w, r)
			return
		}
		if r.Method == http.MethodPost {
			if strings.Contains(r.URL.RawQuery, "passwd") {
				t.Errorf("password leaked into the login URL: %s", r.URL.RawQuery)
			}
			_ = r.ParseForm()
			if r.PostForm.Get("api") != "SYNO.API.Auth" || r.PostForm.Get("method") != "login" || r.PostForm.Get("enable_syno_token") != "yes" {
				t.Errorf("unexpected login form: %v", r.PostForm)
			}
			if r.PostForm.Get("account") != "admin" || r.PostForm.Get("passwd") != "pw" {
				_, _ = w.Write([]byte(`{"success":false,"error":{"code":400}}`))
				return
			}
			n := atomic.AddInt32(&logins, 1)
			_, _ = w.Write([]byte(`{"success":true,"data":{"sid":"sid` + string(rune('0'+n)) + `","synotoken":"tok","did":"dev-1"}}`))
			return
		}
		q := r.URL.Query()
		if q.Get("_sid") == "" || r.Header.Get("X-SYNO-TOKEN") != "tok" {
			_, _ = w.Write([]byte(`{"success":false,"error":{"code":119}}`))
			return
		}
		if expireFirst && atomic.AddInt32(&dataCalls, 1) == 1 {
			_, _ = w.Write([]byte(`{"success":false,"error":{"code":119}}`))
			return
		}
		if q.Get("api") == "SYNO.ActiveBackup.Task" {
			_, _ = w.Write([]byte(`{"success":true,"data":{"tasks":[{"task_id":5}]}}`))
			return
		}
		_, _ = w.Write([]byte(`{"success":false,"error":{"code":102}}`))
	}))
	t.Cleanup(srv.Close)
	return srv, &logins
}

func TestLoginParsesSession(t *testing.T) {
	srv, _ := fakeNAS(t, false)
	s, err := Login(context.Background(), srv.Client(), srv.URL, Credentials{Account: "admin", Password: "pw"})
	if err != nil {
		t.Fatal(err)
	}
	if s.SID != "sid1" || s.SynoToken != "tok" || s.DeviceID != "dev-1" {
		t.Fatalf("session = %+v", s)
	}
}

func TestLoginRejectsBadPassword(t *testing.T) {
	srv, _ := fakeNAS(t, false)
	_, err := Login(context.Background(), srv.Client(), srv.URL, Credentials{Account: "admin", Password: "wrong"})
	var de *Error
	if !errors.As(err, &de) || de.Code != 400 || !strings.Contains(err.Error(), "invalid account or password") {
		t.Fatalf("err = %v", err)
	}
}

func TestLoginNeedsCredentials(t *testing.T) {
	if _, err := Login(context.Background(), http.DefaultClient, "https://nas", Credentials{}); err == nil {
		t.Fatal("expected an error without credentials")
	}
}

func TestClientCallUnwrapsAndRelogsIn(t *testing.T) {
	srv, logins := fakeNAS(t, true)
	c := &Client{HTTP: srv.Client(), BaseURL: srv.URL, Creds: Credentials{Account: "admin", Password: "pw"}}
	data, err := c.Get(context.Background(), "/webapi/entry.cgi?api=SYNO.ActiveBackup.Task&method=list&version=1", url.Values{"load_result": {"true"}})
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != `{"tasks":[{"task_id":5}]}` {
		t.Fatalf("data = %s", data)
	}
	if *logins != 2 {
		t.Fatalf("logins = %d, want 2 (relogin after code 119)", *logins)
	}
}

func TestClientCallSurfacesDSMError(t *testing.T) {
	srv, _ := fakeNAS(t, false)
	c := &Client{HTTP: srv.Client(), BaseURL: srv.URL, Creds: Credentials{Account: "admin", Password: "pw"}}
	_, err := c.Get(context.Background(), "/webapi/entry.cgi?api=SYNO.Backup.Task&method=list&version=1", nil)
	var de *Error
	if !errors.As(err, &de) || de.Code != 102 {
		t.Fatalf("err = %v, want DSM error 102", err)
	}
}

func TestGetRejectsNonEnvelope(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if dsmtest.ServeAPIInfo(w, r) {
			return
		}
		if r.Method == http.MethodPost {
			_, _ = w.Write([]byte(`{"success":true,"data":{"sid":"s","synotoken":"t"}}`))
			return
		}
		if r.URL.Query().Get("api") == "SYNO.Bare" {
			_, _ = w.Write([]byte(`{"success":true}`))
			return
		}
		_, _ = w.Write([]byte(`<html><body>Portal</body></html>`))
	}))
	defer srv.Close()
	c := &Client{HTTP: srv.Client(), BaseURL: srv.URL, Creds: Credentials{Account: "a", Password: "p"}, RatePerSec: 1000}
	for _, path := range []string{"/webapi/entry.cgi?api=SYNO.Html&method=get&version=1", "/webapi/entry.cgi?api=SYNO.Bare&method=get&version=1"} {
		if _, err := c.Get(context.Background(), path, nil); err == nil || !strings.Contains(err.Error(), "data envelope") {
			t.Fatalf("%s: err = %v, want a non-envelope error", path, err)
		}
	}
}

func TestLoginFallsBackFromRejectedDeviceTokenToOTP(t *testing.T) {
	var forms []url.Values
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if dsmtest.ServeAPIInfo(w, r) {
			return
		}
		_ = r.ParseForm()
		forms = append(forms, r.PostForm)
		switch {
		case r.PostForm.Get("device_id") == "stale-did":
			_, _ = w.Write([]byte(`{"success":false,"error":{"code":403}}`))
		case r.PostForm.Get("otp_code") == "123456" && r.PostForm.Get("device_id") == "":
			_, _ = w.Write([]byte(`{"success":true,"data":{"sid":"s","synotoken":"t","did":"fresh-did"}}`))
		default:
			_, _ = w.Write([]byte(`{"success":false,"error":{"code":400}}`))
		}
	}))
	defer srv.Close()
	s, err := Login(context.Background(), srv.Client(), srv.URL, Credentials{Account: "a", Password: "p", DeviceID: "stale-did", OTPCode: "123456"})
	if err != nil || s.DeviceID != "fresh-did" {
		t.Fatalf("session=%+v err=%v", s, err)
	}
	if len(forms) != 2 || forms[0].Get("otp_code") != "" {
		t.Fatalf("expected device-token attempt then OTP attempt, got %v", forms)
	}
}

func TestReloginUsesDeviceTokenNotSpentOTP(t *testing.T) {
	var logins []url.Values
	expired := true
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if dsmtest.ServeAPIInfo(w, r) {
			return
		}
		if r.Method == http.MethodPost {
			_ = r.ParseForm()
			logins = append(logins, r.PostForm)
			_, _ = w.Write([]byte(`{"success":true,"data":{"sid":"s","synotoken":"t","did":"did-1"}}`))
			return
		}
		if expired {
			expired = false
			_, _ = w.Write([]byte(`{"success":false,"error":{"code":119}}`))
			return
		}
		_, _ = w.Write([]byte(`{"success":true,"data":{"ok":true}}`))
	}))
	defer srv.Close()
	c := &Client{HTTP: srv.Client(), BaseURL: srv.URL, Creds: Credentials{Account: "a", Password: "p", OTPCode: "123456"}, RatePerSec: 1000}
	if _, err := c.Get(context.Background(), "/webapi/entry.cgi?api=SYNO.X&method=get&version=1", nil); err != nil {
		t.Fatal(err)
	}
	if len(logins) != 2 || logins[1].Get("otp_code") != "" || logins[1].Get("device_id") != "did-1" {
		t.Fatalf("relogin forms = %v", logins)
	}
}

func TestLoginRefusesRedirect(t *testing.T) {
	var leaked bool
	evil := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		leaked = r.PostForm.Get("passwd") != ""
		_, _ = w.Write([]byte(`{"success":true,"data":{"sid":"s"}}`))
	}))
	defer evil.Close()
	nas := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if dsmtest.ServeAPIInfo(w, r) {
			return
		}
		http.Redirect(w, r, evil.URL+r.URL.Path, http.StatusTemporaryRedirect)
	}))
	defer nas.Close()
	if _, err := Login(context.Background(), nas.Client(), nas.URL, Credentials{Account: "a", Password: "testpass0"}); err == nil {
		t.Fatal("login followed a redirect")
	}
	if leaked {
		t.Fatal("password re-sent to the redirect target")
	}
}

func TestSnippetNeverQuotesBody(t *testing.T) {
	if s := snippet([]byte(`{"_sid":"secret-sid"}`)); strings.Contains(s, "secret") {
		t.Fatalf("snippet leaked body: %s", s)
	}
}

func TestEnvelopeFailureWithOddErrorCode(t *testing.T) {
	for body, want := range map[string]int{
		`{"success":false,"error":{"code":"119"}}`: 119,
		`{"success":false,"error":{"code":"x"}}`:   100,
		`{"success":false,"error":"nope"}`:         100,
		`{"success":"false"}`:                      100,
	} {
		_, code, failed := Envelope([]byte(body))
		if !failed || code != want {
			t.Errorf("%s: failed=%v code=%d, want failed with %d", body, failed, code, want)
		}
	}
}

func TestSameOriginRedirectComparesEffectiveOrigins(t *testing.T) {
	via := func(u string) []*http.Request {
		r, _ := http.NewRequest(http.MethodGet, u, nil)
		return []*http.Request{r}
	}
	next := func(u string) *http.Request {
		r, _ := http.NewRequest(http.MethodGet, u, nil)
		return r
	}
	if SameOriginRedirect(next("https://nas/webapi"), via("http://nas/webapi")) == nil {
		t.Error("http to https on the same host was treated as one origin")
	}
	if SameOriginRedirect(next("https://NAS:443/x"), via("https://nas/webapi")) != nil {
		t.Error("an explicit default port was treated as another origin")
	}
}

func TestScrubEncodedSecrets(t *testing.T) {
	in := `Get "https://sso/login?return=https%3A%2F%2Fnas%2Fwebapi%3Fapi%3Dx%26_sid%3DSECRET123%26a%3Db": refused`
	if out := Scrub(in); strings.Contains(out, "SECRET123") {
		t.Fatalf("encoded sid survived: %s", out)
	}
	if out := scrubSecrets("redirect to /x/SECRET456/y", "SECRET456"); strings.Contains(out, "SECRET456") {
		t.Fatalf("known sid survived: %s", out)
	}
	if out := Scrub("_sid=abc&keep=1"); out != "_sid=***&keep=1" {
		t.Fatalf("plain scrub = %q", out)
	}
}

func TestScrubKnownValueBeforeParameters(t *testing.T) {
	out := scrubSecrets("passwd=alpha%26OMEGASECRET&x=1", "alpha&OMEGASECRET")
	if strings.Contains(out, "OMEGA") {
		t.Fatalf("suffix of a known secret leaked: %s", out)
	}
}

func TestScrubLongestSecretFirstAndJSONForms(t *testing.T) {
	if out := scrubSecrets("refused: alphaOMEGASECRET", "alpha", "alphaOMEGASECRET"); strings.Contains(out, "OMEGA") {
		t.Fatalf("a shorter secret broke a longer one: %s", out)
	}
	if out := scrubSecrets(`{"error":"bad quote\"secret"}`, `quote"secret`); strings.Contains(out, "secret") {
		t.Fatalf("JSON-escaped secret survived: %s", out)
	}
}

func TestScrubShortAndEncodedSecrets(t *testing.T) {
	if out := scrubSecrets("redirect to /echo/abc", "abc"); strings.Contains(out, "abc") {
		t.Fatalf("a short secret survived: %s", out)
	}
	if out := scrubSecrets("to /echo/alpha%2526OMEGASECRET", "alpha&OMEGASECRET"); strings.Contains(out, "OMEGA") {
		t.Fatalf("a doubly encoded secret survived: %s", out)
	}
}

func TestDecodedMaskingCoversAnyEncoding(t *testing.T) {
	for _, in := range []string{
		"title alpha&#38;#38;OMEGASECRET",
		"redirect /echo/alpha%2fOMEGASECRET",
		"redirect /echo/alpha%252fOMEGASECRET",
	} {
		if out := scrubSecrets(in, "alpha&OMEGASECRET", "alpha/OMEGASECRET"); strings.Contains(out, "OMEGA") {
			t.Errorf("%q leaked: %s", in, out)
		}
	}
}

func TestDecodedFixedPointAndLiteralEscapeSecrets(t *testing.T) {
	deep := "alpha&#38;" + strings.Repeat("#38;", 12) + "OMEGASECRET"
	if out := scrubSecrets(deep, "alpha&OMEGASECRET"); strings.Contains(out, "OMEGA") {
		t.Errorf("deeply nested entities leaked: %s", out)
	}
	if out := scrubSecrets("echo alpha/OMEGASECRET", "alpha%2fOMEGASECRET"); strings.Contains(out, "OMEGA") {
		t.Errorf("a secret containing a literal escape leaked in decoded form: %s", out)
	}
}

func TestReadErrorsDoNotQuoteTheBody(t *testing.T) {
	if err := errReadBody("x"); strings.Contains(err.Error(), "%") {
		t.Fatal(err)
	}
}

func TestTransportErrorsDropRedirectURL(t *testing.T) {
	evil := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	defer evil.Close()
	nas := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if dsmtest.ServeAPIInfo(w, r) {
			return
		}
		if r.Method == http.MethodPost {
			_, _ = w.Write([]byte(`{"success":true,"data":{"sid":"SIDSECRET34","synotoken":"TOKSECRET34"}}`))
			return
		}
		http.Redirect(w, r, evil.URL+"/echo/PASSSECRET34/DEVICESECRET34/SIDSECRET34", http.StatusFound)
	}))
	defer nas.Close()
	hc := nas.Client()
	hc.CheckRedirect = SameOriginRedirect
	c := &Client{HTTP: hc, BaseURL: nas.URL, Creds: Credentials{Account: "a", Password: "PASSSECRET34", DeviceID: "DEVICESECRET34"}, RatePerSec: 1000}
	_, err := c.Get(context.Background(), "/webapi/entry.cgi?api=SYNO.ActiveBackup.Task&method=list&version=1", nil)
	if err == nil || strings.Contains(err.Error(), "SECRET34") {
		t.Fatalf("err = %v", err)
	}
}

func TestRedactCoversEveryCredentialUsed(t *testing.T) {
	nas := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if dsmtest.ServeAPIInfo(w, r) {
			return
		}
		_, _ = w.Write([]byte(`{"success":true,"data":{"sid":"SIDSECRET35","synotoken":"TOKSECRET35","did":"NEWDEVICE35"}}`))
	}))
	defer nas.Close()
	c := &Client{HTTP: nas.Client(), BaseURL: nas.URL, Creds: Credentials{Account: "a", Password: "987654321035", OTPCode: "123456"}, RatePerSec: 1000}
	if _, err := c.session(context.Background()); err != nil {
		t.Fatal(err)
	}
	out := c.Redact("task 987654321035 listed twice; otp 123456; SIDSECRET35 TOKSECRET35 NEWDEVICE35")
	for _, s := range []string{"987654321035", "123456", "SIDSECRET35", "TOKSECRET35", "NEWDEVICE35"} {
		if strings.Contains(out, s) {
			t.Errorf("%s survived: %s", s, out)
		}
	}
	if got := describeTransport(fmt.Errorf("malformed HTTP status code %q", "PASS")); strings.Contains(got, "PASS") {
		t.Errorf("transport description quoted the response: %s", got)
	}
}
