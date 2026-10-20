// Hand-written for saas-alerts (not generated): Respond access-token mint tests.

package client

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"saas-alerts-pp-cli/internal/config"
)

func newRespondTestClient(t *testing.T, h http.HandlerFunc) *Client {
	t.Helper()
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	c := New(&config.Config{BaseURL: srv.URL, SaasAlertsApiKey: "partner-key"}, 5*time.Second, 0)
	c.NoCache = true
	return c
}

func TestRespondTokenMintedAndReMintedOnce(t *testing.T) {
	var mints, calls atomic.Int32
	c := newRespondTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case respondAuthPath:
			if r.Method != http.MethodPost || r.Header.Get("api_key") != "partner-key" {
				t.Errorf("mint: method=%s api_key=%q", r.Method, r.Header.Get("api_key"))
			}
			n := mints.Add(1)
			w.Write([]byte(`{"accessToken":"tok-` + string(rune('0'+n)) + `"}`))
		case "/respondApi/api/v1/states":
			calls.Add(1)
			if r.Header.Get("accesstoken") != "tok-2" { // first token is "expired"
				w.WriteHeader(http.StatusBadRequest)
				w.Write([]byte("Only secure origins are allowed"))
				return
			}
			w.Write([]byte(`{"enabled":true}`))
		default:
			t.Errorf("unexpected path %s", r.URL.Path)
		}
	})
	out, err := c.Get(context.Background(), "/respondApi/api/v1/states", nil)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if string(out) != `{"enabled":true}` || mints.Load() != 2 || calls.Load() != 2 {
		t.Fatalf("out=%s mints=%d calls=%d", out, mints.Load(), calls.Load())
	}
}

func TestRespondTokenRejectedTwiceSurfacesError(t *testing.T) {
	var mints atomic.Int32
	c := newRespondTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == respondAuthPath {
			mints.Add(1)
			w.Write([]byte(`{"accessToken":"tok"}`))
			return
		}
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte(`Access token is invalid`))
	})
	if _, err := c.Get(context.Background(), "/respondApi/api/v1/rules", nil); err == nil {
		t.Fatal("want error after one re-mint")
	}
	if mints.Load() != 2 {
		t.Fatalf("mints=%d, want 2 (initial + one re-mint)", mints.Load())
	}
}

func TestNonRespondPathsSendNoAccessToken(t *testing.T) {
	c := newRespondTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == respondAuthPath || r.Header.Get("accesstoken") != "" {
			t.Errorf("Reports call minted or sent accesstoken: %s", r.URL.Path)
		}
		if r.Header.Get("api_key") != "partner-key" {
			t.Errorf("api_key=%q", r.Header.Get("api_key"))
		}
		w.Write([]byte(`[]`))
	})
	if _, err := c.Get(context.Background(), "/reportApi/api/v1/customers", nil); err != nil {
		t.Fatalf("Get: %v", err)
	}
}

func TestRespondMintFailureNamesTheCause(t *testing.T) {
	c := newRespondTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte("Not Authorized"))
	})
	_, err := c.Get(context.Background(), "/respondApi/api/v1/states", nil)
	if err == nil || !strings.Contains(err.Error(), "Respond access-token mint") {
		t.Fatalf("err=%v", err)
	}
}

func TestRespondTokenMaskedWhenErrorBodyEchoesIt(t *testing.T) {
	var reached atomic.Int32
	c := newRespondTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == respondAuthPath {
			w.Write([]byte(`{"accessToken":"secret-minted-token-123"}`))
			return
		}
		reached.Add(1)
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(`{"error":"bad request","accesstoken":"` + r.Header.Get("accesstoken") + `"}`))
	})
	_, err := c.Get(context.Background(), "/respondApi/api/v1/states", nil)
	if err == nil || reached.Load() != 1 {
		t.Fatalf("want one request that errors; reached=%d err=%v", reached.Load(), err)
	}
	// Only the token value matters; short tokens minted by sibling tests may also
	// mask fragments of the key name.
	if strings.Contains(err.Error(), "secret-minted-token-123") {
		t.Fatalf("token not masked in error: %v", err)
	}
}

func TestRespondTokenReplacedByReMintIsStillMasked(t *testing.T) {
	var mints atomic.Int32
	c := newRespondTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == respondAuthPath {
			n := mints.Add(1)
			w.Write([]byte(`{"accessToken":"old-token-` + string(rune('0'+n)) + `"}`))
			return
		}
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(`echo old-token-1`))
	})
	if _, err := c.respondAccessToken(context.Background(), "partner-key", false); err != nil {
		t.Fatal(err)
	}
	if _, err := c.respondAccessToken(context.Background(), "partner-key", true); err != nil {
		t.Fatal(err)
	}
	if got := c.maskCredentialText("echo old-token-1 and old-token-2"); strings.Contains(got, "old-token-1") || strings.Contains(got, "old-token-2") {
		t.Fatalf("a minted token survived masking: %q", got)
	}
}

func TestRespondMintTransportErrorIsMasked(t *testing.T) {
	var mints atomic.Int32
	c := newRespondTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == respondAuthPath {
			if mints.Add(1) == 1 {
				w.Write([]byte(`{"accessToken":"earlier-minted-token-456"}`))
				return
			}
			// The refresh redirects to an unreachable URL carrying both secrets.
			http.Redirect(w, r, "http://127.0.0.1:1/x?k=partner-key&t=earlier-minted-token-456", http.StatusFound)
			return
		}
		w.Write([]byte(`{}`))
	})
	if _, err := c.respondAccessToken(context.Background(), "partner-key", false); err != nil {
		t.Fatal(err)
	}
	_, err := c.respondAccessToken(context.Background(), "partner-key", true)
	if err == nil {
		t.Fatal("want a mint transport error")
	}
	if msg := err.Error(); strings.Contains(msg, "partner-key") || strings.Contains(msg, "earlier-minted-token-456") {
		t.Fatalf("mint error leaked a credential: %v", msg)
	}
}

// TestRespondTokenAndKeyEchoedEscapedAreMasked: a Respond token echoed
// JSON-escaped in an API error, and the partner key echoed JSON-escaped by a
// failed mint, are masked like the plain forms.
func TestRespondTokenAndKeyEchoedEscapedAreMasked(t *testing.T) {
	c := newRespondTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == respondAuthPath {
			w.Write([]byte(`{"accessToken":"secret/minted+token-456"}`))
			return
		}
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(`{"error":"token secret\/minted\u002Btoken-456 refused"}`))
	})
	if _, err := c.Get(context.Background(), "/respondApi/api/v1/states", nil); err == nil || strings.Contains(err.Error(), "minted") {
		t.Fatalf("escaped Respond token echo leaked: %v", err)
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte(`{"error":"key partner\/key\u002B0000 is not valid"}`))
	}))
	t.Cleanup(srv.Close)
	k := New(&config.Config{BaseURL: srv.URL, SaasAlertsApiKey: "partner/key+0000"}, 5*time.Second, 0)
	k.NoCache = true
	if _, err := k.Get(context.Background(), "/respondApi/api/v1/states", nil); err == nil || strings.Contains(err.Error(), "key+0000") || strings.Contains(err.Error(), `key\u002B0000`) {
		t.Fatalf("escaped partner key echo in the mint error leaked: %v", err)
	}
}
