// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// HAND-AUTHORED. Pins dsm.VerifyDSM (probe before any session or credential)
// and dsm.OriginJar (cookies bound to one origin).

package dsm

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"synology-pp-cli/internal/dsm/dsmtest"
)

// gateHost logs every request it receives (method, URL, Cookie) and
// answers it with h.
func gateHost(t *testing.T, h http.HandlerFunc) (*httptest.Server, func() []string) {
	t.Helper()
	var mu sync.Mutex
	var log []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		log = append(log, r.Method+" "+r.URL.String()+" cookie="+r.Header.Get("Cookie")+" token="+r.Header.Get("X-SYNO-TOKEN"))
		mu.Unlock()
		h(w, r)
	}))
	t.Cleanup(srv.Close)
	return srv, func() []string { mu.Lock(); defer mu.Unlock(); return append([]string(nil), log...) }
}

func routerPage(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/html")
	_, _ = io.WriteString(w, "<html><body>router login</body></html>")
}

// TestVerifyDSMProbesOncePerAddressPerProcess: the SYNO.API.Info probe runs
// once per NAS address per process, however many requests (and goroutines,
// and spellings of the address) follow; a host that is not DSM is never
// remembered as verified, so it is probed again and refused again.
func TestVerifyDSMProbesOncePerAddressPerProcess(t *testing.T) {
	ForgetVerifiedAddresses()
	var probes atomic.Int32
	nas := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if dsmtest.ServeAPIInfo(w, r) {
			probes.Add(1)
		}
	}))
	defer nas.Close()
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := VerifyDSM(context.Background(), nas.Client(), nas.URL); err != nil {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
	for _, spelling := range []string{nas.URL + "/", strings.Replace(nas.URL, "http://", "HTTP://", 1)} {
		if err := VerifyDSM(context.Background(), nas.Client(), spelling); err != nil {
			t.Fatal(err)
		}
	}
	if n := probes.Load(); n != 1 {
		t.Fatalf("the address was probed %d times, want once per process", n)
	}

	router, seen := gateHost(t, routerPage)
	for i := 0; i < 2; i++ {
		err := VerifyDSM(context.Background(), router.Client(), router.URL)
		var notDSM *NotDSMError
		if !errors.As(err, &notDSM) {
			t.Fatalf("a router page was accepted as DSM: %v", err)
		}
		refused := SessionNotSent(err)
		if !strings.Contains(refused.Error(), "the session was not sent") || !errors.As(refused, &notDSM) {
			t.Fatalf("refusal = %v", refused)
		}
	}
	if n := len(seen()); n != 2 {
		t.Fatalf("a host that is not DSM was remembered (%d probes for 2 checks): %v", n, seen())
	}
}

// TestProbeCarriesNoCookie: the probe goes to a host before it has shown it
// is a DSM, so it carries no cookie even when the caller's client has a jar
// holding one for that origin.
func TestProbeCarriesNoCookie(t *testing.T) {
	ForgetVerifiedAddresses()
	host, seen := gateHost(t, routerPage)
	jar, _ := cookiejar.New(nil)
	u, _ := url.Parse(host.URL)
	jar.SetCookies(u, []*http.Cookie{{Name: "id", Value: "COOKIE-SENTINEL-55"}})
	hc := &http.Client{Jar: jar}
	_ = ProbeDSM(context.Background(), hc, host.URL)
	_ = VerifyDSM(context.Background(), hc, host.URL)
	for _, l := range seen() {
		if strings.Contains(l, "COOKIE-SENTINEL-55") {
			t.Fatalf("the probe carried the session cookie: %s", l)
		}
	}
	if len(seen()) == 0 {
		t.Fatal("no probe was sent; the check proved nothing")
	}
}

// TestFleetClientNeverSendsASessionToANonDSM: the fleet client attaches the
// sid and SynoToken only for an address that answered SYNO.API.Info as DSM
// in this process; a session it holds (from any source) is refused for a
// host that is not DSM, on reads and on logout, and nothing but the probe
// reaches that host.
func TestFleetClientNeverSendsASessionToANonDSM(t *testing.T) {
	ForgetVerifiedAddresses()
	router, seen := gateHost(t, routerPage)
	c := &Client{HTTP: router.Client(), BaseURL: router.URL, Creds: Credentials{Account: "a", Password: "PW-FLEET-1"}, RatePerSec: 1000}
	c.sess = Session{SID: "SID-FLEET-SENTINEL", SynoToken: "TOK-FLEET-SENTINEL"}
	_, err := c.Get(context.Background(), "/webapi/entry.cgi?api=SYNO.ActiveBackup.Task&method=list&version=1", nil)
	var notDSM *NotDSMError
	if !errors.As(err, &notDSM) || !strings.Contains(err.Error(), "the session was not sent") {
		t.Fatalf("Get = %v, want a refusal naming the host as not DSM", err)
	}
	c.sess = Session{SID: "SID-FLEET-SENTINEL", SynoToken: "TOK-FLEET-SENTINEL"}
	if err := c.Logout(context.Background()); !errors.As(err, &notDSM) {
		t.Fatalf("Logout = %v, want a refusal", err)
	}
	for _, l := range seen() {
		if strings.Contains(l, "SENTINEL") || !strings.Contains(l, "SYNO.API.Info") {
			t.Fatalf("a host that is not DSM received more than the probe: %s", l)
		}
	}
}

// TestOriginJarBindsCookiesToOneOrigin: the session manager's jar keeps and
// returns cookies for exactly its NAS origin (scheme, host and port), marks
// an https:// origin's cookies Secure, and Bind starts it empty.
func TestOriginJarBindsCookiesToOneOrigin(t *testing.T) {
	mustURL := func(s string) *url.URL { u, _ := url.Parse(s); return u }
	j := NewOriginJar()
	j.SetCookies(mustURL("https://nas:5001/"), []*http.Cookie{{Name: "id", Value: "UNBOUND"}})
	if got := j.Cookies(mustURL("https://nas:5001/")); len(got) != 0 {
		t.Fatalf("an unbound jar kept a cookie: %v", got)
	}
	j.Bind("https://NAS:5001")
	j.SetCookies(mustURL("https://nas:5001/webapi/entry.cgi"), []*http.Cookie{{Name: "id", Value: "SECURE-1", Path: "/"}})
	j.SetCookies(mustURL("https://nas:5002/"), []*http.Cookie{{Name: "other", Value: "PORT"}})
	j.SetCookies(mustURL("http://nas:5001/"), []*http.Cookie{{Name: "plain", Value: "SCHEME"}})
	if got := j.Cookies(mustURL("https://nas:5001/webapi/entry.cgi")); len(got) != 1 || got[0].Value != "SECURE-1" {
		t.Fatalf("bound origin cookies = %v, want only SECURE-1", got)
	}
	for _, other := range []string{"http://nas:5001/", "http://nas:5000/", "https://nas:5002/", "https://other:5001/"} {
		if got := j.Cookies(mustURL(other)); len(got) != 0 {
			t.Fatalf("%s got the session cookie: %v", other, got)
		}
	}
	j.Bind("https://nas:5001")
	if got := j.Cookies(mustURL("https://nas:5001/")); len(got) != 0 {
		t.Fatalf("Bind kept a cookie: %v", got)
	}
	// An https:// origin's cookies are stored Secure: even the underlying
	// host-scoped jar would not hand them to http:// on that host.
	j.Bind("https://nas:443")
	j.SetCookies(mustURL("https://nas/"), []*http.Cookie{{Name: "id", Value: "S2"}})
	if got := j.Cookies(mustURL("http://nas:443/")); len(got) != 0 {
		t.Fatalf("an https cookie went to http: %v", got)
	}
	if got := j.jar.Cookies(mustURL("http://nas/")); len(got) != 0 {
		t.Fatalf("an https cookie was stored without Secure: %v", got)
	}
	if got := j.Cookies(mustURL("https://nas/")); len(got) != 1 {
		t.Fatalf("default port spelling lost the cookie: %v", got)
	}
}

// TestMaskingIsTypePreservingAndCoversShortSecrets: every non-empty secret
// is masked when it is an entire string value, whatever its length; inside
// larger text only secrets of 8 or more characters are; a number, boolean
// or null is never rewritten (a one-time code equal to a result_id leaves
// the id a number); an object key is never renamed for a short secret.
func TestMaskingIsTypePreservingAndCoversShortSecrets(t *testing.T) {
	const pw, otp = "abc", "482913"
	body := `{"users":[{"name":"alice","description":"abc","note":" abc ","text":"xabcx","abc":1}],` +
		`"last_result":{"result_id":482913,"code":"482913","ok":true,"none":null,"n":-482913.5},"total":2}`
	out := string(MaskResponse([]byte(body), pw, otp))
	if !strings.Contains(out, `"description":"[redacted]"`) || !strings.Contains(out, `"note":"[redacted]"`) {
		t.Fatalf("a 3-character password that is an entire field survived: %s", out)
	}
	if !strings.Contains(out, `"code":"[redacted]"`) {
		t.Fatalf("a string echo of the one-time code survived: %s", out)
	}
	for _, keep := range []string{`"result_id":482913`, `"ok":true`, `"none":null`, `"n":-482913.5`, `"text":"xabcx"`, `"abc":1`, `"total":2`} {
		if !strings.Contains(out, keep) {
			t.Fatalf("masking changed %s (types and keys must be kept): %s", keep, out)
		}
	}
	var decoded struct {
		LastResult struct {
			ResultID int64 `json:"result_id"`
		} `json:"last_result"`
	}
	if err := json.Unmarshal([]byte(out), &decoded); err != nil || decoded.LastResult.ResultID != 482913 {
		t.Fatalf("a typed parser cannot read the masked answer: %v %s", err, out)
	}
	if got := string(MaskResponse([]byte(`{"a":"x"}`), "", "  ")); got != `{"a":"x"}` {
		t.Fatalf("an empty secret masked something: %s", got)
	}
}
