// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package client

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"
)

// Golden vector from https://duo.com/docs/adminapi#authentication (v5 POST example).
func TestDuoSignV5GoldenVector(t *testing.T) {
	got := duoSignV5("DIWJ8X6AEYOR5OMC6TQ1", "Zh5eGmUq9zpfQnyUIu5OL9iWoMMv5ZNmk3zLJ4Ep", "POST",
		"api-xxxxxxxx.duosecurity.com", "/admin/v1/users", url.Values{},
		[]byte(`{"realname":"First Last","username":"root"}`), "Tue, 21 Aug 2012 17:29:18 -0000")
	want := "Basic RElXSjhYNkFFWU9SNU9NQzZUUTE6MzIxM2RhNjdkM2NiOGNmY2ZlNWJiMTgyMWI3NjJmY2ZlYzI1OTg1NmE0Yjc1OWY4NDI4NzhmZjkzNDA0NjcyYjI4NGNiZGQxNDQwZDQxNzgyYzQxODllYWVmNmZjMzgzNzg2MzBhZjc1Y2VkNzlkMGM5NDExMGYwNGI2MzEyZjg="
	if got != want {
		t.Fatalf("signature mismatch\n got %s\nwant %s", got, want)
	}
}

func TestDuoCanonHeaders(t *testing.T) {
	if duoCanonHeaders(http.Header{"Accept": {"x"}}) != sha512Hex(nil) {
		t.Fatal("non X-Duo headers must not be signed")
	}
	h := http.Header{"X-Duo-B": {" two "}, "X-Duo-A": {"one"}}
	if got, want := duoCanonHeaders(h), sha512Hex([]byte("x-duo-a\x00one\x00x-duo-b\x00two")); got != want {
		t.Fatalf("canonical X-Duo headers: got %s want %s", got, want)
	}
}

func TestDuoCanonParams(t *testing.T) {
	v := url.Values{"username": {"root"}, "realname": {"First Last"}, "z": {"b", "a~"}}
	if got, want := duoCanonParams(v), "realname=First%20Last&username=root&z=a~&z=b"; got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}

type captureRT struct{ reqs []*http.Request }

// resetAccountCache isolates tests from the process-lifetime account cache.
func resetAccountCache(t *testing.T) {
	t.Helper()
	duoAccounts.Lock()
	duoAccounts.byKey = map[string]*duoAccountEntry{}
	duoAccounts.Unlock()
}

func (c *captureRT) RoundTrip(r *http.Request) (*http.Response, error) {
	c.reqs = append(c.reqs, r)
	body := `{"stat":"OK","response":{}}`
	if r.URL.Path == duoAccountsListPath {
		body = `{"stat":"OK","response":[{"account_id":"DA9VZOC5X63I2W72NRP9","name":"Acme","api_hostname":"api-child123.duosecurity.com"}]}`
	}
	return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(body)), Header: http.Header{}}, nil
}

func basicHeader(ikey, skey string) string {
	return "Basic " + base64.StdEncoding.EncodeToString([]byte(ikey+":"+skey))
}

func TestDuoSignerSignsAndNeverSendsSecret(t *testing.T) {
	resetAccountCache(t)
	rt := &captureRT{}
	s := newDuoSigner(rt)
	s.now = func() time.Time { return time.Date(2026, 10, 13, 12, 0, 0, 0, time.UTC) }
	req, _ := http.NewRequest(http.MethodGet, "https://api-parent.duosecurity.com/admin/v1/users?username=First+Last", nil)
	req.Header.Set("Authorization", basicHeader("DIKEY", "SECRETKEY"))
	if _, err := s.RoundTrip(req); err != nil {
		t.Fatal(err)
	}
	out := rt.reqs[0]
	if strings.Contains(out.Header.Get("Authorization"), base64.StdEncoding.EncodeToString([]byte("DIKEY:SECRETKEY"))) {
		t.Fatal("secret key sent verbatim")
	}
	want := duoSignV5("DIKEY", "SECRETKEY", "GET", "api-parent.duosecurity.com", "/admin/v1/users",
		url.Values{"username": {"First Last"}}, nil, out.Header.Get("Date"))
	if out.Header.Get("Authorization") != want || out.URL.RawQuery != "username=First%20Last" {
		t.Fatalf("bad signing: auth=%s query=%s", out.Header.Get("Authorization"), out.URL.RawQuery)
	}
}

func TestDuoSignerRoutesSubaccount(t *testing.T) {
	resetAccountCache(t)
	rt := &captureRT{}
	s := newDuoSigner(rt)
	req, _ := http.NewRequest(http.MethodPost, "https://api-parent.duosecurity.com/admin/v1/users/DU1/bypass_codes",
		strings.NewReader(`{"account_id":"DA9VZOC5X63I2W72NRP9","count":"1"}`))
	req.Header.Set("Authorization", basicHeader("DIKEY", "SECRETKEY"))
	if _, err := s.RoundTrip(req); err != nil {
		t.Fatal(err)
	}
	if len(rt.reqs) != 2 || rt.reqs[0].URL.Path != duoAccountsListPath || rt.reqs[0].URL.Host != "api-parent.duosecurity.com" {
		t.Fatalf("expected account lookup on parent first, got %d requests", len(rt.reqs))
	}
	out := rt.reqs[1]
	if out.URL.Host != "api-child123.duosecurity.com" {
		t.Fatalf("not routed to child host: %s", out.URL.Host)
	}
	b, _ := io.ReadAll(out.Body)
	var m map[string]string
	_ = json.Unmarshal(b, &m)
	if m["account_id"] != "DA9VZOC5X63I2W72NRP9" || out.Header.Get("Content-Type") != "application/json" {
		t.Fatalf("body/content-type wrong: %s %s", b, out.Header.Get("Content-Type"))
	}
	// Second call hits the cache: no extra lookup.
	req2, _ := http.NewRequest(http.MethodGet, "https://api-parent.duosecurity.com/admin/v1/users?account_id=DA9VZOC5X63I2W72NRP9", nil)
	req2.Header.Set("Authorization", basicHeader("DIKEY", "SECRETKEY"))
	if _, err := s.RoundTrip(req2); err != nil {
		t.Fatal(err)
	}
	if len(rt.reqs) != 3 || rt.reqs[2].URL.Host != "api-child123.duosecurity.com" {
		t.Fatalf("cache miss or wrong host: %d", len(rt.reqs))
	}
}

func TestDuoSignerEnvPinAndEmptyPost(t *testing.T) {
	resetAccountCache(t)
	t.Setenv(DuoAccountEnv, "DA9VZOC5X63I2W72NRP9")
	rt := &captureRT{}
	s := newDuoSigner(rt)
	req, _ := http.NewRequest(http.MethodPost, "https://api-parent.duosecurity.com/admin/v1/admins/DE1/reset", nil)
	req.Header.Set("Authorization", basicHeader("DIKEY", "SECRETKEY"))
	if _, err := s.RoundTrip(req); err != nil {
		t.Fatal(err)
	}
	out := rt.reqs[len(rt.reqs)-1]
	b, _ := io.ReadAll(out.Body)
	if string(b) != `{"account_id":"DA9VZOC5X63I2W72NRP9"}` || out.URL.Host != "api-child123.duosecurity.com" {
		t.Fatalf("env pin not applied: %s host=%s", b, out.URL.Host)
	}
	// /accounts/v1/* is never rerouted or injected.
	rt.reqs = nil
	req2, _ := http.NewRequest(http.MethodPost, "https://api-parent.duosecurity.com/accounts/v1/account/list", nil)
	req2.Header.Set("Authorization", basicHeader("DIKEY", "SECRETKEY"))
	if _, err := s.RoundTrip(req2); err != nil {
		t.Fatal(err)
	}
	b2, _ := io.ReadAll(rt.reqs[0].Body)
	if string(b2) != "{}" || rt.reqs[0].URL.Host != "api-parent.duosecurity.com" {
		t.Fatalf("accounts call altered: %s %s", b2, rt.reqs[0].URL.Host)
	}
}

func TestDuoSignerResolvesSubaccountName(t *testing.T) {
	resetAccountCache(t)
	rt := &captureRT{}
	s := newDuoSigner(rt)
	req, _ := http.NewRequest(http.MethodGet, "https://api-parent.duosecurity.com/admin/v1/users?account_id=acme", nil)
	req.Header.Set("Authorization", basicHeader("DIKEY", "SECRETKEY"))
	if _, err := s.RoundTrip(req); err != nil {
		t.Fatal(err)
	}
	out := rt.reqs[len(rt.reqs)-1]
	if out.URL.Query().Get("account_id") != "DA9VZOC5X63I2W72NRP9" || out.URL.Host != "api-child123.duosecurity.com" {
		t.Fatalf("name not resolved: %s %s", out.URL.RawQuery, out.URL.Host)
	}
	req2, _ := http.NewRequest(http.MethodGet, "https://api-parent.duosecurity.com/admin/v1/users?account_id=nope", nil)
	req2.Header.Set("Authorization", basicHeader("DIKEY", "SECRETKEY"))
	resp, err := s.RoundTrip(req2)
	if err != nil || resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected a 400 response, got %v %v", resp, err)
	}
	b, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(b), "not found") || !strings.Contains(string(b), `"stat":"FAIL"`) {
		t.Fatalf("expected Duo-shaped not-found body, got %s", b)
	}
}

type ambiguousRT struct{ captureRT }

func (a *ambiguousRT) RoundTrip(r *http.Request) (*http.Response, error) {
	a.reqs = append(a.reqs, r)
	body := `{"stat":"OK","response":[{"account_id":"DA1","name":"Acme","api_hostname":"api-a.duosecurity.com"},{"account_id":"DA2","name":"ACME","api_hostname":"api-b.duosecurity.com"}]}`
	return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(body)), Header: http.Header{}}, nil
}

func TestDuoSignerAmbiguousNameAndNullBody(t *testing.T) {
	resetAccountCache(t)
	s := newDuoSigner(&ambiguousRT{})
	req, _ := http.NewRequest(http.MethodGet, "https://api-parent.duosecurity.com/admin/v1/users?account_id=acme", nil)
	req.Header.Set("Authorization", basicHeader("DIKEY", "SECRETKEY"))
	resp, err := s.RoundTrip(req)
	if err != nil || resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("ambiguous name must be a 400, got %v %v", resp, err)
	}
	if b, _ := io.ReadAll(resp.Body); !strings.Contains(string(b), "matches 2 accounts") {
		t.Fatalf("ambiguity message missing: %s", b)
	}
	req, _ = http.NewRequest(http.MethodGet, "https://api-parent.duosecurity.com/admin/v1/users?account_id=da2", nil)
	req.Header.Set("Authorization", basicHeader("DIKEY", "SECRETKEY"))
	if _, err := s.RoundTrip(req); err != nil {
		t.Fatalf("ID lookup is case-insensitive: %v", err)
	}
	t.Setenv(DuoAccountEnv, "DA2")
	req, _ = http.NewRequest(http.MethodPost, "https://api-parent.duosecurity.com/admin/v1/admins/DE1/reset", strings.NewReader("null"))
	req.Header.Set("Authorization", basicHeader("DIKEY", "SECRETKEY"))
	if _, err := s.RoundTrip(req); err != nil {
		t.Fatalf("null body with pin must not fail: %v", err)
	}
}

func TestDuoAccountCacheSharedAndRefetchedOnMiss(t *testing.T) {
	resetAccountCache(t)
	rt := &captureRT{}
	for i := 0; i < 2; i++ { // two signers = two MCP calls: one list call total
		req, _ := http.NewRequest(http.MethodGet, "https://api-parent.duosecurity.com/admin/v1/users?account_id=DA9VZOC5X63I2W72NRP9", nil)
		req.Header.Set("Authorization", basicHeader("DIKEY", "SECRETKEY"))
		if _, err := newDuoSigner(rt).RoundTrip(req); err != nil {
			t.Fatal(err)
		}
	}
	lists := 0
	for _, r := range rt.reqs {
		if r.URL.Path == duoAccountsListPath {
			lists++
		}
	}
	if lists != 1 {
		t.Fatalf("expected one shared account-list call, got %d", lists)
	}
	req, _ := http.NewRequest(http.MethodGet, "https://api-parent.duosecurity.com/admin/v1/users?account_id=DANEW0000000000000001", nil)
	req.Header.Set("Authorization", basicHeader("DIKEY", "SECRETKEY"))
	if _, err := newDuoSigner(rt).RoundTrip(req); err != nil {
		t.Fatal(err)
	}
	lists = 0
	for _, r := range rt.reqs {
		if r.URL.Path == duoAccountsListPath {
			lists++
		}
	}
	if lists != 2 {
		t.Fatalf("a miss on the cached list must refetch exactly once, got %d list calls", lists)
	}
}

type switchRT struct {
	captureRT
	body string
}

func (s *switchRT) RoundTrip(r *http.Request) (*http.Response, error) {
	s.reqs = append(s.reqs, r)
	return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(s.body)), Header: http.Header{}}, nil
}

func TestDuoSignerNameAmbiguityNotHiddenByCache(t *testing.T) {
	resetAccountCache(t)
	rt := &switchRT{body: `{"stat":"OK","response":[{"account_id":"DA1","name":"Acme","api_hostname":"api-a.duosecurity.com"}]}`}
	req, _ := http.NewRequest(http.MethodGet, "https://api-parent.duosecurity.com/admin/v1/users?account_id=acme", nil)
	req.Header.Set("Authorization", basicHeader("DIKEY", "SECRETKEY"))
	if resp, err := newDuoSigner(rt).RoundTrip(req); err != nil || resp.StatusCode != 200 {
		t.Fatalf("first resolution: %v %v", resp, err)
	}
	rt.body = `{"stat":"OK","response":[{"account_id":"DA1","name":"Acme","api_hostname":"api-a.duosecurity.com"},{"account_id":"DA2","name":"ACME","api_hostname":"api-b.duosecurity.com"}]}`
	req, _ = http.NewRequest(http.MethodGet, "https://api-parent.duosecurity.com/admin/v1/users?account_id=acme", nil)
	req.Header.Set("Authorization", basicHeader("DIKEY", "SECRETKEY"))
	resp, err := newDuoSigner(rt).RoundTrip(req) // a new MCP call
	if err != nil || resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("a new same-named account must make the name ambiguous, got %v %v", resp, err)
	}
}

func TestDuoSignerIDShapedNameNotHiddenByCache(t *testing.T) {
	resetAccountCache(t)
	const name = "DAAAAAAAAAAAAAAAAAAA" // a name that looks like an account ID
	rt := &switchRT{body: `{"stat":"OK","response":[{"account_id":"DA1","name":"` + name + `","api_hostname":"api-a.duosecurity.com"}]}`}
	sign := func() *http.Response {
		req, _ := http.NewRequest(http.MethodGet, "https://api-parent.duosecurity.com/admin/v1/users?account_id="+name, nil)
		req.Header.Set("Authorization", basicHeader("DIKEY", "SECRETKEY"))
		resp, err := newDuoSigner(rt).RoundTrip(req)
		if err != nil {
			t.Fatal(err)
		}
		return resp
	}
	if resp := sign(); resp.StatusCode != 200 {
		t.Fatalf("first resolution: %d", resp.StatusCode)
	}
	rt.body = `{"stat":"OK","response":[{"account_id":"DA1","name":"` + name + `","api_hostname":"api-a.duosecurity.com"},{"account_id":"DA2","name":"` + name + `","api_hostname":"api-b.duosecurity.com"}]}`
	if resp := sign(); resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("an ID-shaped name must still be checked against a fresh list, got %d", resp.StatusCode)
	}
}

func TestDuoSignerNeverForwardsUnsignableCredentials(t *testing.T) {
	resetAccountCache(t)
	rt := &captureRT{}
	req, _ := http.NewRequest(http.MethodGet, "https://api-parent.duosecurity.com/admin/v1/users", nil)
	req.Header.Set("Authorization", basicHeader("DIKEY", "SECRETKEY")+" ")
	if _, err := newDuoSigner(rt).RoundTrip(req); err != nil {
		t.Fatal(err)
	}
	if got := rt.reqs[0].Header.Get("Authorization"); got == basicHeader("DIKEY", "SECRETKEY") || !strings.HasPrefix(got, "Basic ") || rt.reqs[0].Header.Get("Date") == "" {
		t.Fatalf("whitespace-padded credentials must still be signed, got %q", got)
	}
	req, _ = http.NewRequest(http.MethodGet, "https://api-parent.duosecurity.com/admin/v1/users", nil)
	req.Header.Set("Authorization", "Basic !!not-base64!!")
	if _, err := newDuoSigner(rt).RoundTrip(req); err != nil {
		t.Fatal(err)
	}
	if got := rt.reqs[1].Header.Get("Authorization"); got != "" {
		t.Fatalf("an unsignable Authorization header must be dropped, got %q", got)
	}
}

func TestDuoAccountLoadHonorsContext(t *testing.T) {
	resetAccountCache(t)
	release := make(chan struct{})
	go func() {
		_, _, _ = duoAccountSetFor(context.Background(), "slow|K", func(context.Context) ([]DuoSubaccount, error) {
			<-release
			return nil, nil
		})
	}()
	time.Sleep(20 * time.Millisecond)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	if _, _, err := duoAccountSetFor(ctx, "slow|K", nil); err == nil {
		t.Fatal("a waiter must give up when its own context ends")
	}
	if _, _, err := duoAccountSetFor(context.Background(), "other|K", func(context.Context) ([]DuoSubaccount, error) { return nil, nil }); err != nil {
		t.Fatalf("another parent must not wait on the slow load: %v", err)
	}
	close(release)
}

func TestDuoSignerUnsignedWithoutCredentials(t *testing.T) {
	resetAccountCache(t)
	rt := &captureRT{}
	req, _ := http.NewRequest(http.MethodGet, "https://api-parent.duosecurity.com/admin/v1/users", nil)
	if _, err := newDuoSigner(rt).RoundTrip(req); err != nil {
		t.Fatal(err)
	}
	if rt.reqs[0].Header.Get("Authorization") != "" {
		t.Fatal("unexpected auth header")
	}
}

func TestPinnedAccountIDIgnoresUnresolvedPlaceholder(t *testing.T) {
	t.Setenv(DuoAccountEnv, "${user_config.cisco_duo_account_id}")
	if got := PinnedAccountID(); got != "" {
		t.Fatalf("an unresolved MCPB placeholder must read as unset, got %q", got)
	}
	t.Setenv(DuoAccountEnv, " DA9VZOC5X63I2W72NRP9 ")
	if got := PinnedAccountID(); got != "DA9VZOC5X63I2W72NRP9" {
		t.Fatalf("a real pin must survive, got %q", got)
	}
}

// twoAccountRT is a parent with two subaccounts, Acme (DA..A) and Bravo
// (DB..B), each on its own API host.
type twoAccountRT struct{ captureRT }

func (a *twoAccountRT) RoundTrip(r *http.Request) (*http.Response, error) {
	a.reqs = append(a.reqs, r)
	body := `{"stat":"OK","response":{}}`
	if r.URL.Path == duoAccountsListPath {
		body = `{"stat":"OK","response":[{"account_id":"DAAAAAAAAAAAAAAAAAAA","name":"Acme","api_hostname":"api-acme.duosecurity.com"},{"account_id":"DBBBBBBBBBBBBBBBBBBB","name":"Bravo","api_hostname":"api-bravo.duosecurity.com"}]}`
	}
	return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(body)), Header: http.Header{}}, nil
}

// sentTo reports whether any non-lookup request went to host.
func (a *twoAccountRT) sentTo(host string) bool {
	for _, r := range a.reqs {
		if r.URL.Path != duoAccountsListPath && r.URL.Host == host {
			return true
		}
	}
	return false
}

func signedReq(t *testing.T, method, target, body string) *http.Request {
	t.Helper()
	var rd io.Reader
	if body != "" {
		rd = strings.NewReader(body)
	}
	req, err := http.NewRequest(method, target, rd)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", basicHeader("DIKEY", "SECRETKEY"))
	return req
}

// Codex merge review (P1): the pin selects the local store, so a request that
// names another subaccount (a --stdin body, an import line, an MCP argument)
// reached that customer while its response was stored as the pinned one's.
func TestDuoSignerRequestAccountMustMatchPin(t *testing.T) {
	resetAccountCache(t)
	t.Setenv(DuoAccountEnv, "DAAAAAAAAAAAAAAAAAAA")
	rt := &twoAccountRT{}
	s := newDuoSigner(rt)
	for _, req := range []*http.Request{
		signedReq(t, http.MethodPost, "https://api-parent.duosecurity.com/admin/v1/users/bulk_create", `{"account_id":"DBBBBBBBBBBBBBBBBBBB","users":[]}`),
		signedReq(t, http.MethodPost, "https://api-parent.duosecurity.com/admin/v1/users", `{"account_id":"Bravo","username":"b"}`),
		signedReq(t, http.MethodGet, "https://api-parent.duosecurity.com/admin/v1/users?account_id=DBBBBBBBBBBBBBBBBBBB", ""),
		signedReq(t, http.MethodDelete, "https://api-parent.duosecurity.com/admin/v1/users/DU1?account_id=bravo", ""),
	} {
		resp, err := s.RoundTrip(req)
		if err != nil || resp.StatusCode != http.StatusBadRequest {
			t.Fatalf("%s %s naming another subaccount under a pin must be a 400, got %v %v", req.Method, req.URL, resp, err)
		}
		if b, _ := io.ReadAll(resp.Body); !strings.Contains(string(b), "pinned") {
			t.Fatalf("refusal must say the run is pinned: %s", b)
		}
	}
	if rt.sentTo("api-bravo.duosecurity.com") || rt.sentTo("api-acme.duosecurity.com") {
		t.Fatal("a refused request must not be sent to any tenant")
	}
	// Naming the pinned subaccount itself (by ID or by name) is fine and is
	// sent with the resolved ID.
	for _, req := range []*http.Request{
		signedReq(t, http.MethodPost, "https://api-parent.duosecurity.com/admin/v1/users", `{"account_id":"acme","username":"a"}`),
		signedReq(t, http.MethodGet, "https://api-parent.duosecurity.com/admin/v1/users?account_id=daaaaaaaaaaaaaaaaaaa", ""),
	} {
		resp, err := s.RoundTrip(req)
		if err != nil || resp.StatusCode != http.StatusOK {
			t.Fatalf("%s naming the pinned subaccount must pass: %v %v", req.Method, resp, err)
		}
		out := rt.reqs[len(rt.reqs)-1]
		if out.URL.Host != "api-acme.duosecurity.com" {
			t.Fatalf("not routed to the pinned subaccount: %s", out.URL.Host)
		}
	}
}

// TestDuoSignerPinBindingSeesThePin: the store-binding hook is told about the
// PIN (the store's key), even when the request names the same account by ID
// while the pin is a name, so a moved name is still caught.
func TestDuoSignerPinBindingSeesThePin(t *testing.T) {
	resetAccountCache(t)
	t.Setenv(DuoAccountEnv, "Acme")
	var calls []string
	AccountResolvedHook = func(requested, accountID string) error {
		calls = append(calls, requested+"="+accountID)
		return nil
	}
	defer func() { AccountResolvedHook = nil }()
	s := newDuoSigner(&twoAccountRT{})
	if resp, err := s.RoundTrip(signedReq(t, http.MethodGet, "https://api-parent.duosecurity.com/admin/v1/users?account_id=DAAAAAAAAAAAAAAAAAAA", "")); err != nil || resp.StatusCode != http.StatusOK {
		t.Fatalf("same account by ID must pass: %v %v", resp, err)
	}
	if len(calls) != 1 || calls[0] != "Acme=DAAAAAAAAAAAAAAAAAAA" {
		t.Fatalf("the binding hook must see the pin and its resolved ID, got %v", calls)
	}
}

func TestDuoSignerUnpinnedGuard(t *testing.T) {
	resetAccountCache(t)
	t.Setenv(DuoAccountEnv, "")
	var asked []string
	UnpinnedAccountGuard = func(requested string) error {
		asked = append(asked, requested)
		return errors.New("refused for the test")
	}
	defer func() { UnpinnedAccountGuard = nil }()
	rt := &twoAccountRT{}
	s := newDuoSigner(rt)
	resp, err := s.RoundTrip(signedReq(t, http.MethodPost, "https://api-parent.duosecurity.com/admin/v1/users/bulk_create", `{"account_id":"Bravo","users":[]}`))
	if err != nil || resp.StatusCode != http.StatusBadRequest || len(asked) != 1 || asked[0] != "Bravo" {
		t.Fatalf("an unpinned request naming a subaccount must ask the guard and honor its refusal: %v %v %v", resp, err, asked)
	}
	if len(rt.reqs) != 0 {
		t.Fatalf("a guard refusal must not send anything (not even the lookup), sent %d", len(rt.reqs))
	}
	// A request that names no subaccount never asks the guard.
	if _, err := s.RoundTrip(signedReq(t, http.MethodGet, "https://api-parent.duosecurity.com/admin/v1/users", "")); err != nil || len(asked) != 1 {
		t.Fatalf("parent request must not consult the guard: %v %v", err, asked)
	}
	// With no guard (the MCP server's execute tool, which stores nothing) an
	// unpinned request still routes per request.
	UnpinnedAccountGuard = nil
	if resp, err := s.RoundTrip(signedReq(t, http.MethodGet, "https://api-parent.duosecurity.com/admin/v1/users?account_id=Bravo", "")); err != nil || resp.StatusCode != http.StatusOK || !rt.sentTo("api-bravo.duosecurity.com") {
		t.Fatalf("unguarded per-request routing must still work: %v %v", resp, err)
	}
}

// TestDuoSignerAccountIDShapes: values the guard cannot compare must not
// slip past it to the parent account.
func TestDuoSignerAccountIDShapes(t *testing.T) {
	resetAccountCache(t)
	t.Setenv(DuoAccountEnv, "DAAAAAAAAAAAAAAAAAAA")
	rt := &twoAccountRT{}
	s := newDuoSigner(rt)
	for _, body := range []string{`{"account_id":123}`, `{"account_id":["DBBBBBBBBBBBBBBBBBBB"]}`, `["not","an","object"]`} {
		resp, err := s.RoundTrip(signedReq(t, http.MethodPost, "https://api-parent.duosecurity.com/admin/v1/users", body))
		if err != nil || resp.StatusCode != http.StatusBadRequest {
			t.Fatalf("body %s under a pin must be refused, got %v %v", body, resp, err)
		}
	}
	resp, err := s.RoundTrip(signedReq(t, http.MethodGet, "https://api-parent.duosecurity.com/admin/v1/users?account_id=DAAAAAAAAAAAAAAAAAAA&account_id=DBBBBBBBBBBBBBBBBBBB", ""))
	if err != nil || resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("a repeated account_id must be refused, got %v %v", resp, err)
	}
	if rt.sentTo("api-parent.duosecurity.com") || rt.sentTo("api-bravo.duosecurity.com") {
		t.Fatal("an uncomparable account_id reached the parent or another tenant")
	}
	// null and empty count as absent: the pin is applied.
	for _, body := range []string{`{"account_id":null,"username":"a"}`, `{"account_id":"","username":"a"}`} {
		resp, err := s.RoundTrip(signedReq(t, http.MethodPost, "https://api-parent.duosecurity.com/admin/v1/users", body))
		if err != nil || resp.StatusCode != http.StatusOK {
			t.Fatalf("body %s must take the pin: %v %v", body, resp, err)
		}
		out := rt.reqs[len(rt.reqs)-1]
		b, _ := io.ReadAll(out.Body)
		if out.URL.Host != "api-acme.duosecurity.com" || !strings.Contains(string(b), `"account_id":"DAAAAAAAAAAAAAAAAAAA"`) {
			t.Fatalf("pin not applied for %s: host=%s body=%s", body, out.URL.Host, b)
		}
	}
}
