// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"bytes"
	"compress/zlib"
	"context"
	"encoding/pem"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"comet-backup-pp-cli/internal/client"
	"comet-backup-pp-cli/internal/cliutil"
	"comet-backup-pp-cli/internal/cliutil/testenv"
	"comet-backup-pp-cli/internal/credkit"
)

// init runs this package's unit tests without the credential kit's
// outbound guard (see client.OutboundGuard): no test here runs credkit.Start, so
// the guard would refuse every credentialed request to the loopback servers
// these tests stand up. The guard is proven on the built binaries
// (internal/cometblackbox).
func init() {
	client.OutboundGuard = func(rt http.RoundTripper) http.RoundTripper { return rt }
	// A value held in-process stays held for the whole test binary and would
	// withhold later tests' ordinary statements; heldIn captures it instead.
	cliutil.Hold = func(credkit.Source, ...string) {}
}

// api-origin-pinned: a redirect to another host never reaches it, so the
// admin password riding in the form body is never sent there; the configured
// server still answers.
func TestOriginPinRefusesRedirectToAnotherHost(t *testing.T) {
	if inFreshProcess(t) {
		return
	}
	var hits, ok atomic.Int32
	other := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		_, _ = w.Write([]byte(`{}`))
	}))
	t.Cleanup(other.Close)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/admin/meta/version" {
			ok.Add(1)
			_, _ = w.Write([]byte(`{"Version":"24.9.1"}`))
			return
		}
		http.Redirect(w, r, other.URL+r.URL.Path, http.StatusTemporaryRedirect)
	}))
	t.Cleanup(srv.Close)
	testenv.Isolate(t)
	t.Setenv("COMET_BACKUP_BASE_URL", srv.URL)
	t.Setenv("COMET_BACKUP_USERNAME", "admin")
	t.Setenv("COMET_BACKUP_PASSWORD", "pinUnitAdminPass-4c1e")
	flags := rootFlags{}
	c, err := flags.newClient()
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := c.PostForm(context.Background(), "/api/v1/admin/meta/version", url.Values{}); err != nil || ok.Load() != 1 {
		t.Fatalf("the configured server must still answer: hits=%d err=%v", ok.Load(), err)
	}
	_, _, err = c.PostForm(context.Background(), "/api/v1/admin/list-users", url.Values{})
	if hits.Load() != 0 {
		t.Fatalf("the redirect target was contacted %d time(s)", hits.Load())
	}
	if err == nil || !strings.Contains(err.Error(), "leave the configured Comet Server") {
		t.Fatalf("want the pin refusal, got %v", err)
	}
}

// The pin follows the host https://{server} resolves to from
// COMET_BACKUP_SERVER (no base URL override): requests there go through,
// a redirect off it does not.
func TestOriginPinFollowsTemplatedServer(t *testing.T) {
	if inFreshProcess(t) {
		return
	}
	var hits, ok atomic.Int32
	other := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
	}))
	t.Cleanup(other.Close)
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/admin/meta/version" {
			ok.Add(1)
			_, _ = w.Write([]byte(`{"Version":"24.9.1"}`))
			return
		}
		http.Redirect(w, r, other.URL+r.URL.Path, http.StatusTemporaryRedirect)
	}))
	t.Cleanup(srv.Close)
	home := testenv.Isolate(t)
	ca := filepath.Join(home, "ca.pem")
	if err := os.WriteFile(ca, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: srv.Certificate().Raw}), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("COMET_BACKUP_BASE_URL", "")
	t.Setenv("COMET_BACKUP_SERVER", strings.TrimPrefix(srv.URL, "https://"))
	t.Setenv("COMET_BACKUP_CA_CERT", ca)
	t.Setenv("COMET_BACKUP_USERNAME", "admin")
	t.Setenv("COMET_BACKUP_PASSWORD", "pinTmplPass-8a2d")
	flags := rootFlags{}
	c, err := flags.newClient()
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := c.PostForm(context.Background(), "/api/v1/admin/meta/version", url.Values{}); err != nil || ok.Load() != 1 {
		t.Fatalf("the templated server must answer: hits=%d err=%v", ok.Load(), err)
	}
	_, _, err = c.PostForm(context.Background(), "/api/v1/admin/list-users", url.Values{})
	if hits.Load() != 0 || err == nil || !strings.Contains(err.Error(), "leave the configured Comet Server") {
		t.Fatalf("redirect off the templated server: hits=%d err=%v", hits.Load(), err)
	}
}

func TestNormalizedOriginRefusesNonASCIIAndFillsPorts(t *testing.T) {
	for raw, want := range map[string]string{
		"https://Comet.Example.com/api":     "https://comet.example.com:443",
		"https://comet.example.com:8060/x":  "https://comet.example.com:8060",
		"http://127.0.0.1/x":                "http://127.0.0.1:80",
		"https://İ.example.com/x":           "",
		"https://comet.example.com:8060/x/": "https://comet.example.com:8060",
	} {
		u, _ := url.Parse(raw)
		if got := normalizedOrigin(u); got != want {
			t.Errorf("%s: got %q, want %q", raw, got, want)
		}
	}
	got := pinnedOrigins(client.ResolvedBaseURL("https://{server}", map[string]string{"server": "comet.example.com:8060"}), "://bad")
	if len(got) != 1 || !got["https://comet.example.com:8060"] {
		t.Errorf("pinned origins: %v", got)
	}
	if got := pinnedOrigins(client.ResolvedBaseURL("https://{server}", nil)); got["https://:443"] {
		t.Errorf("an unresolved template pinned an empty host: %v", got)
	}
}

// kitHolds reports whether the credential kit holds v now: a file write of v
// alone is withheld.
func kitHolds(t *testing.T, v string) bool {
	t.Helper()
	err := credkit.WriteFile(filepath.Join(t.TempDir(), "probe"), []byte(v), 0o600)
	if err != nil && !errors.Is(err, credkit.ErrWithheld) {
		t.Fatal(err)
	}
	return errors.Is(err, credkit.ErrWithheld)
}

// response-credentials-held: a credential a response carries is held and
// replaced by name before any command reads the body, deflate-encoded or not, at
// any depth, even one that reads like a mask; fields that are not credentials
// are neither held nor replaced.
func TestResponseCredentialsHeld(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/admin/storage/list-buckets" {
			var z bytes.Buffer
			zw := zlib.NewWriter(&z)
			_, _ = zw.Write([]byte(`{"b1":{"ReadWriteKey":"bucketRwKeyUnitDeflate77","OrganizationID":"org-1"}}`))
			_ = zw.Close()
			w.Header().Set("Content-Encoding", "deflate")
			_, _ = w.Write(z.Bytes())
			return
		}
		_, _ = w.Write([]byte(`{"alice":{"Username":"alice","PasswordHash":"pwHashUnitF6g7h8i9","Destinations":{"d1":{"Description":"descNotSecretK2m4","S3SecretKey":"s3SecretUnitA1b2c3","SFTP":{"SFTPPassword":"sftpPassUnitZ9y8"}}},"Password":"**********"}}`))
	}))
	t.Cleanup(srv.Close)
	rt := responseHoldTripper{next: http.DefaultTransport}
	bodies := ""
	for _, p := range []string{"/api/v1/admin/storage/list-buckets", "/api/v1/admin/list-users-full"} {
		req, _ := http.NewRequest(http.MethodPost, srv.URL+p, nil)
		resp, err := rt.RoundTrip(req)
		if err != nil {
			t.Fatal(err)
		}
		body, _ := io.ReadAll(resp.Body)
		if len(body) == 0 {
			t.Fatalf("%s: body not handed on", p)
		}
		bodies += string(body)
	}
	for _, v := range []string{"bucketRwKeyUnitDeflate77", "pwHashUnitF6g7h8i9", "s3SecretUnitA1b2c3", "sftpPassUnitZ9y8", "**********"} {
		if strings.Contains(bodies, v) {
			t.Errorf("%s handed on to the command", v)
		}
	}
	for _, v := range []string{"descNotSecretK2m4", "org-1", "alice"} {
		if kitHolds(t, v) {
			t.Errorf("%s held, but it is not a credential", v)
		}
		if !strings.Contains(bodies, v) {
			t.Errorf("%s lost from the body", v)
		}
	}
}

// Codex r1 (publish): a feedback endpoint answering 307 to another host with
// the credential in the Location was followed. Every off-host sender goes
// through sendOffHost, which never follows a redirect.
func TestOffHostSendersNeverFollowRedirects(t *testing.T) {
	const pw = "RedirectOnlyPassword42"
	var hits atomic.Int32
	collector := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { hits.Add(1) }))
	t.Cleanup(collector.Close)
	for _, code := range []int{301, 302, 303, 307, 308} {
		redirector := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			http.Redirect(w, r, collector.URL+"/?password="+pw, code)
		}))
		err := postFeedback(redirector.URL, FeedbackEntry{Text: "ok"})
		if err == nil || !strings.Contains(err.Error(), "redirects are not followed") {
			t.Errorf("feedback %d: err=%v, want a refused redirect", code, err)
		}
		err = deliverWebhook(redirector.URL, []byte(`{}`), false)
		if err == nil || !strings.Contains(err.Error(), "redirects are not followed") {
			t.Errorf("webhook %d: err=%v, want a refused redirect", code, err)
		}
		redirector.Close()
	}
	if hits.Load() != 0 {
		t.Fatalf("%d request(s) followed a redirect to another host", hits.Load())
	}
}
