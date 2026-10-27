// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

// Hand-fix (handfixes.json vault-secrets-withheld): proves Hudu password-vault
// secrets never leave the client, on every verb, on encoded paths, from the
// response cache, and that a body the filter cannot classify is an error.

package client

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"hudu-pp-cli/internal/config"
)

const (
	vaultSentinelPW  = "SENTINEL-PW-1"
	vaultSentinelOTP = "SENTINEL-OTP-1"
)

// vaultEntryJSON is one entry as Hudu returns it: metadata plus the password
// and the TOTP seed in plaintext, plus a key the allowlist has never seen and
// a timestamp key outside the exact allowlist (rotated_at).
const vaultEntryJSON = `{"id":7,"company_id":3,"company_name":"Acme","name":"DC admin","username":"administrator","url":"https://dc.acme.test/?a=1&b=2","password":"` + vaultSentinelPW + `","otp_secret":"` + vaultSentinelOTP + `","vendor_new_field":"` + vaultSentinelPW + `","password_folder_name":"Infra","archived":false,"created_at":"2026-01-02T03:04:05Z","updated_at":"2026-02-03T04:05:06Z","rotated_at":"2026-02-03T04:05:06Z"}`

// companiesRaw is a non-vault body with odd spacing and HTML characters; it
// must reach the caller byte for byte.
const companiesRaw = `{ "companies" : [ {"id":1,"name":"A&B <Co>","password":"not-a-vault-field"} ] ,"meta":{"page":1}}`

func decodeUntilStable(p string) string {
	for {
		next, err := url.PathUnescape(p)
		if err != nil || next == p {
			return strings.ToLower(p)
		}
		p = next
	}
}

// newVaultStub serves the vault shapes Hudu returns and a few it should never
// return, routed on the fully decoded path.
func newVaultStub(t *testing.T) (*httptest.Server, *int64) {
	t.Helper()
	var hits int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt64(&hits, 1)
		p := decodeUntilStable(r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		switch {
		case p == "/companies":
			_, _ = w.Write([]byte(companiesRaw))
		case p == "/asset_passwords" && r.Method == http.MethodPost:
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`{"asset_password":` + vaultEntryJSON + `}`))
		case p == "/asset_passwords":
			_, _ = w.Write([]byte(`{"asset_passwords":[` + vaultEntryJSON + `,` + vaultEntryJSON + `],"meta":{"current_page":1,"total_pages":1}}`))
		case p == "/asset_passwords/7" && r.Method == http.MethodDelete:
			w.WriteHeader(http.StatusNoContent)
		case p == "/asset_passwords/7":
			_, _ = w.Write([]byte(`{"asset_password":` + vaultEntryJSON + `}`))
		case p == "/asset_passwords/bare-array":
			_, _ = w.Write([]byte(`[` + vaultEntryJSON + `]`))
		case p == "/asset_passwords/bare-object":
			_, _ = w.Write([]byte(vaultEntryJSON))
		case p == "/asset_passwords/not-json":
			w.Header().Set("Content-Type", "text/plain")
			_, _ = w.Write([]byte("password=" + vaultSentinelPW + "\n"))
		case p == "/asset_passwords/scalar":
			_, _ = w.Write([]byte(`"` + vaultSentinelPW + `"`))
		case p == "/asset_passwords/binary":
			w.Header().Set("Content-Type", "application/octet-stream")
			_, _ = w.Write([]byte(vaultSentinelPW))
		case p == "/asset_passwords/sibling-secret":
			_, _ = w.Write([]byte(`{"asset_password":` + vaultEntryJSON + `,"included":[{"otp_secret":"` + vaultSentinelOTP + `"}]}`))
		case p == "/asset_passwords/echo-error":
			w.WriteHeader(http.StatusUnprocessableEntity)
			_, _ = w.Write([]byte(`{"error":"invalid","asset_password":` + vaultEntryJSON + `}`))
		case p == "/asset_passwords/validation-error":
			w.WriteHeader(http.StatusUnprocessableEntity)
			_, _ = w.Write([]byte(`{"errors":{"password":["can't be blank"]}}`))
		default:
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"error":"not found"}`))
		}
	}))
	t.Cleanup(srv.Close)
	return srv, &hits
}

func newVaultTestClient(t *testing.T, baseURL string) *Client {
	t.Helper()
	t.Setenv("PRINTING_PRESS_VERIFY", "")
	t.Setenv("PRINTING_PRESS_VERIFY_LIVE_HTTP", "")
	// Keep New's cache sweep off the operator's real cache dir.
	t.Setenv("HUDU_HOME", t.TempDir())
	c := New(&config.Config{BaseURL: baseURL}, 5*time.Second, 0)
	c.cacheDir = t.TempDir()
	c.NoCache = true
	return c
}

func assertNoVaultSecret(t *testing.T, label string, data []byte) {
	t.Helper()
	for _, s := range []string{vaultSentinelPW, vaultSentinelOTP} {
		if strings.Contains(string(data), s) {
			t.Fatalf("%s leaked %s: %s", label, s, data)
		}
	}
}

// assertVaultMetadata decodes one filtered entry and checks the allowlisted
// metadata survived and nothing else did.
func assertVaultMetadata(t *testing.T, label string, raw json.RawMessage) {
	t.Helper()
	var entry map[string]any
	if err := json.Unmarshal(raw, &entry); err != nil {
		t.Fatalf("%s: entry is not an object: %v: %s", label, err, raw)
	}
	if entry["name"] != "DC admin" || entry["username"] != "administrator" || entry["updated_at"] != "2026-02-03T04:05:06Z" {
		t.Fatalf("%s: metadata did not survive: %v", label, entry)
	}
	if entry["url"] != "https://dc.acme.test/?a=1&b=2" || entry["password_folder_name"] != "Infra" {
		t.Fatalf("%s: allowlisted keys did not survive: %v", label, entry)
	}
	// The allowlist is exact: a key ending in _at that is not on it is dropped.
	for _, k := range []string{"password", "otp_secret", "vendor_new_field", "rotated_at"} {
		if _, ok := entry[k]; ok {
			t.Fatalf("%s: %q was not dropped: %v", label, k, entry)
		}
	}
}

func singleEntry(t *testing.T, label string, data json.RawMessage) json.RawMessage {
	t.Helper()
	var env map[string]json.RawMessage
	if err := json.Unmarshal(data, &env); err != nil {
		t.Fatalf("%s: not an object: %v: %s", label, err, data)
	}
	entry, ok := env["asset_password"]
	if !ok {
		t.Fatalf("%s: asset_password envelope missing: %s", label, data)
	}
	return entry
}

func TestVaultWithholdListGet(t *testing.T) {
	srv, _ := newVaultStub(t)
	c := newVaultTestClient(t, srv.URL)
	data, err := c.Get(context.Background(), "/asset_passwords", nil)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	assertNoVaultSecret(t, "GET list", data)
	var env struct {
		AssetPasswords []json.RawMessage `json:"asset_passwords"`
		Meta           map[string]any    `json:"meta"`
	}
	if err := json.Unmarshal(data, &env); err != nil {
		t.Fatalf("list envelope: %v: %s", err, data)
	}
	if len(env.AssetPasswords) != 2 {
		t.Fatalf("list kept %d entries, want 2: %s", len(env.AssetPasswords), data)
	}
	for _, e := range env.AssetPasswords {
		assertVaultMetadata(t, "GET list entry", e)
	}
	if env.Meta["total_pages"] != float64(1) {
		t.Fatalf("pagination meta was not kept: %s", data)
	}
}

func TestVaultWithholdSingleGet(t *testing.T) {
	srv, _ := newVaultStub(t)
	c := newVaultTestClient(t, srv.URL)
	data, err := c.Get(context.Background(), "/asset_passwords/7", nil)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	assertNoVaultSecret(t, "GET single", data)
	assertVaultMetadata(t, "GET single", singleEntry(t, "GET single", data))
}

// TestVaultWithholdEveryVerb drives each wrapper: do (POST, PUT), doRead
// (PostQuery) and doMutation (GetMutating), plus the bare shapes.
func TestVaultWithholdEveryVerb(t *testing.T) {
	srv, _ := newVaultStub(t)
	c := newVaultTestClient(t, srv.URL)
	ctx := context.Background()
	body := map[string]any{"name": "DC admin", "password": "typed-by-operator"}

	data, status, err := c.Post(ctx, "/asset_passwords", body)
	if err != nil || status != http.StatusCreated {
		t.Fatalf("POST create: status=%d err=%v", status, err)
	}
	assertNoVaultSecret(t, "POST create", data)
	assertVaultMetadata(t, "POST create", singleEntry(t, "POST create", data))

	data, _, err = c.Put(ctx, "/asset_passwords/7", body)
	if err != nil {
		t.Fatalf("PUT update: %v", err)
	}
	assertNoVaultSecret(t, "PUT update", data)

	data, _, err = c.PostQueryWithParams(ctx, "/asset_passwords", nil, body)
	if err != nil {
		t.Fatalf("doRead POST: %v", err)
	}
	assertNoVaultSecret(t, "doRead POST", data)

	data, err = c.GetMutating(ctx, "/asset_passwords/7", nil)
	if err != nil {
		t.Fatalf("doMutation GET: %v", err)
	}
	assertNoVaultSecret(t, "doMutation GET", data)

	data, err = c.Get(ctx, "/asset_passwords/bare-array", nil)
	if err != nil {
		t.Fatalf("bare array: %v", err)
	}
	assertNoVaultSecret(t, "bare array", data)
	var arr []json.RawMessage
	if err := json.Unmarshal(data, &arr); err != nil || len(arr) != 1 {
		t.Fatalf("bare array shape lost: %v: %s", err, data)
	}
	assertVaultMetadata(t, "bare array", arr[0])

	data, err = c.Get(ctx, "/asset_passwords/bare-object", nil)
	if err != nil {
		t.Fatalf("bare object: %v", err)
	}
	assertNoVaultSecret(t, "bare object", data)
	assertVaultMetadata(t, "bare object", data)

	data, status, err = c.Delete(ctx, "/asset_passwords/7")
	if err != nil || status != http.StatusNoContent || len(strings.TrimSpace(string(data))) != 0 {
		t.Fatalf("DELETE 204: status=%d err=%v body=%q", status, err, data)
	}
}

func TestVaultWithholdPercentEncodedPath(t *testing.T) {
	srv, _ := newVaultStub(t)
	c := newVaultTestClient(t, srv.URL)
	for _, p := range []string{"/asset%5Fpasswords/7", "/asset%255Fpasswords/7", "/ASSET_PASSWORDS/7", "/asset_passwords/7?company_id=3"} {
		data, err := c.Get(context.Background(), p, nil)
		if err != nil {
			t.Fatalf("%s: %v", p, err)
		}
		assertNoVaultSecret(t, p, data)
		assertVaultMetadata(t, p, singleEntry(t, p, data))
	}
}

// TestVaultWithholdCachedSecondRead proves a vault answer is never cached: a
// cached body keeps no answered URL, so the second read goes to the wire, and
// neither read, nor any file in the cache dir, carries a secret.
func TestVaultWithholdCachedSecondRead(t *testing.T) {
	srv, hits := newVaultStub(t)
	c := newVaultTestClient(t, srv.URL)
	c.NoCache = false
	ctx := context.Background()
	first, err := c.Get(ctx, "/asset_passwords/7", nil)
	if err != nil {
		t.Fatalf("first read: %v", err)
	}
	second, err := c.Get(ctx, "/asset_passwords/7", nil)
	if err != nil {
		t.Fatalf("second read: %v", err)
	}
	if got := atomic.LoadInt64(hits); got != 2 {
		t.Fatalf("a vault answer was served from the cache: %d requests", got)
	}
	assertNoVaultSecret(t, "first read", first)
	assertNoVaultSecret(t, "cached second read", second)
	assertVaultMetadata(t, "cached second read", singleEntry(t, "cached second read", second))
	assertNoVaultSecretOnDisk(t, c.cacheDir)
}

// TestVaultWithholdLegacyCacheEntry plants a plaintext body the way an older
// binary cached it; the cache-read branch must filter it before serving.
func TestVaultWithholdLegacyCacheEntry(t *testing.T) {
	srv, hits := newVaultStub(t)
	c := newVaultTestClient(t, srv.URL)
	c.NoCache = false
	c.writeCacheWithHeaders("/asset_passwords/7", nil, nil, json.RawMessage(`{"asset_password":`+vaultEntryJSON+`}`))
	data, err := c.Get(context.Background(), "/asset_passwords/7", nil)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if got := atomic.LoadInt64(hits); got != 0 {
		t.Fatalf("legacy cache entry was not used: %d requests", got)
	}
	assertNoVaultSecret(t, "legacy cache entry", data)
	assertVaultMetadata(t, "legacy cache entry", singleEntry(t, "legacy cache entry", data))

	// A legacy entry the filter cannot classify is never served: the read
	// goes to the wire instead.
	c.writeCacheWithHeaders("/asset_passwords/7", nil, nil, json.RawMessage(`"`+vaultSentinelPW+`"`))
	data, err = c.Get(context.Background(), "/asset_passwords/7", nil)
	if err != nil {
		t.Fatalf("read past unclassifiable cache entry: %v", err)
	}
	assertNoVaultSecret(t, "unclassifiable legacy cache entry", data)
	if got := atomic.LoadInt64(hits); got != 1 {
		t.Fatalf("unclassifiable cache entry was served: %d requests", got)
	}
}

func assertNoVaultSecretOnDisk(t *testing.T, root string) {
	t.Helper()
	_ = filepath.Walk(root, func(p string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return nil
		}
		b, rerr := os.ReadFile(p)
		if rerr == nil {
			assertNoVaultSecret(t, "file "+p, b)
		}
		return nil
	})
}

// TestVaultWithholdUnclassifiableIsError covers fail-closed: a 2xx body on the
// vault path that is not JSON, a bare JSON scalar, a binary download, or an
// envelope with a secret outside the entries is withheld whole.
func TestVaultWithholdUnclassifiableIsError(t *testing.T) {
	srv, _ := newVaultStub(t)
	c := newVaultTestClient(t, srv.URL)
	for _, p := range []string{"/asset_passwords/not-json", "/asset_passwords/scalar", "/asset_passwords/binary", "/asset_passwords/sibling-secret"} {
		data, err := c.Get(context.Background(), p, nil)
		if err == nil {
			t.Fatalf("%s: want an error, got body %s", p, data)
		}
		if !errors.Is(err, ErrVaultResponseWithheld) {
			t.Fatalf("%s: error does not wrap ErrVaultResponseWithheld: %v", p, err)
		}
		if data != nil {
			t.Fatalf("%s: body returned alongside the refusal: %s", p, data)
		}
		assertNoVaultSecret(t, p+" error", []byte(err.Error()))
	}
}

func TestVaultWithholdErrorBody(t *testing.T) {
	srv, _ := newVaultStub(t)
	c := newVaultTestClient(t, srv.URL)
	_, _, err := c.Post(context.Background(), "/asset_passwords/echo-error", map[string]any{"name": "x"})
	var apiErr *APIError
	if !errors.As(err, &apiErr) || apiErr.StatusCode != http.StatusUnprocessableEntity {
		t.Fatalf("echo error: want a 422 APIError, got %v", err)
	}
	assertNoVaultSecret(t, "echoed error", []byte(err.Error()))

	// Every vault error body is withheld whole, a harmless validation message
	// included: the status code still says what kind of failure it was.
	_, _, err = c.Post(context.Background(), "/asset_passwords/validation-error", map[string]any{"name": "x"})
	if !errors.As(err, &apiErr) || apiErr.StatusCode != http.StatusUnprocessableEntity {
		t.Fatalf("validation error: want a 422 APIError, got %v", err)
	}
	if strings.Contains(err.Error(), "can't be blank") || !strings.Contains(err.Error(), "response body withheld") {
		t.Fatalf("vault error body was not withheld: %v", err)
	}
}

// TestVaultWithholdNonVaultPathByteIdentical proves the filter never touches
// another resource, even one whose body has a "password" key.
func TestVaultWithholdNonVaultPathByteIdentical(t *testing.T) {
	srv, _ := newVaultStub(t)
	c := newVaultTestClient(t, srv.URL)
	data, err := c.Get(context.Background(), "/companies", nil)
	if err != nil {
		t.Fatalf("companies: %v", err)
	}
	if string(data) != companiesRaw {
		t.Fatalf("non-vault body changed:\n got %s\nwant %s", data, companiesRaw)
	}
	data, err = c.Get(context.Background(), "/companies?name=asset_passwords", nil)
	if err != nil || string(data) != companiesRaw {
		t.Fatalf("a query naming the vault changed a non-vault body: %v %s", err, data)
	}
}

// TestVaultWithholdSyntheticEnvelopesPass keeps --dry-run and the verify-mode
// short-circuit working on vault paths.
func TestVaultWithholdSyntheticEnvelopesPass(t *testing.T) {
	srv, hits := newVaultStub(t)
	c := newVaultTestClient(t, srv.URL)
	c.DryRun = true
	data, _, err := c.Post(context.Background(), "/asset_passwords", map[string]any{"name": "x"})
	if err != nil || string(data) != `{"dry_run": true}` {
		t.Fatalf("dry-run envelope changed: %v %s", err, data)
	}
	c.DryRun = false
	t.Setenv("PRINTING_PRESS_VERIFY", "1")
	t.Setenv("PRINTING_PRESS_VERIFY_LIVE_HTTP", "")
	data, _, err = c.Post(context.Background(), "/asset_passwords", map[string]any{"name": "x"})
	if err != nil || !strings.Contains(string(data), "__pp_verify_synthetic__") {
		t.Fatalf("verify envelope changed: %v %s", err, data)
	}
	if got := atomic.LoadInt64(hits); got != 0 {
		t.Fatalf("synthetic paths reached the wire: %d requests", got)
	}
}

func TestIsVaultPath(t *testing.T) {
	cases := map[string]bool{
		"/asset_passwords":                     true,
		"/asset_passwords/7":                   true,
		"/asset_passwords/7/archive":           true,
		"/companies/3/asset_passwords":         true,
		"/asset%5Fpasswords":                   true,
		"/asset%255Fpasswords":                 true,
		"/asset%25255fpasswords":               true,
		"/Asset_Passwords":                     true,
		"/asset%5Fpasswords%zz":                true,
		"/asset_passwords?page=2":              true,
		"/companies":                           false,
		"/companies?type=asset_passwords":      false,
		"/expirations?resource=asset_password": false,
		"/password_folders":                    false,
		"":                                     false,
	}
	for p, want := range cases {
		if got := isVaultPath(p); got != want {
			t.Errorf("isVaultPath(%q) = %v, want %v", p, got, want)
		}
	}
}
