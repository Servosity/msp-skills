// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

package client

import (
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"watchguard-pp-cli/internal/config"
)

// recordingTripper answers 200 and records what each request carried.
type recordingTripper struct{ seen []string }

func (r *recordingTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	r.seen = append(r.seen, req.URL.Host+" auth="+req.Header.Get("Authorization")+" marker="+req.Header.Get(scopedAudienceHeader))
	return &http.Response{StatusCode: 200, Status: "200 OK", Header: http.Header{}, Body: io.NopCloser(strings.NewReader("{}")), Request: req}, nil
}

// seedScoped plants an audience token for c without a token request.
func seedScoped(t *testing.T, c *Client, audience, bearer string) {
	t.Helper()
	scopedMu.Lock()
	scopedTokens[c.scopedKey(audience)] = scopedToken{bearer: bearer, expiry: time.Now().Add(time.Hour)}
	scopedBearers[bearer] = true
	scopedMu.Unlock()
}

// Hand-fix (handfixes.json: scoped-bearer-stays-on-origin, Codex round 1, class
// "credential sent to another host"): the scope transport decides "same
// origin" with the one shared comparator (cliutil.SameOrigin), the way the
// network resolves hosts. Lowercasing the whole origin folded the IPv6 zone,
// so a redirect from fe80::1%lan0 to fe80::1%LAN0 (a different link on Linux)
// had its Authorization stripped by CheckRedirect and then re-stamped here with
// the audience-scoped bearer.
func TestScopeTransportScopedBearerStaysOnOrigin(t *testing.T) {
	cases := []struct {
		name, base, target, pinned, marker string
		wantBearer                         bool
	}{
		{"zone case differs, pinned audience", "https://[fe80::1%25lan0]", "https://[fe80::1%25LAN0]/collect", "aud-zone", "", false},
		{"zone case differs, fan-out marker", "https://[fe80::1%25lan0]", "https://[fe80::1%25LAN0]/collect", "", "aud-zone", false},
		{"other zone, pinned audience", "https://[fe80::1%25lan0]", "https://[fe80::1%25lan1]/collect", "aud-zone", "", false},
		{"non-ASCII look-alike host", "https://k.example", "https://K.example/rest/x", "aud-zone", "", false},
		{"other port", "https://api.usa.cloud.watchguard.com", "https://api.usa.cloud.watchguard.com:8443/rest/x", "aud-zone", "", false},
		// Positive controls: the test can see a bearer when one is sent.
		{"same zone exactly", "https://[fe80::1%25lan0]", "https://[fe80::1%25lan0]/rest/x", "aud-zone", "", true},
		{"DNS name case-insensitive", "https://api.usa.cloud.watchguard.com", "https://API.usa.cloud.watchguard.com/rest/x", "aud-zone", "", true},
		{"default port spelled out", "https://api.usa.cloud.watchguard.com", "https://api.usa.cloud.watchguard.com:443/rest/x", "", "aud-zone", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := &Client{HTTPClient: &http.Client{}, Config: &config.Config{ClientID: "AID-zone-" + tc.name, ClientSecret: "PW", TokenURL: tc.base + "/oauth/token"}}
			bearer := "Bearer tok-zone-" + strings.ReplaceAll(tc.name, " ", "-")
			seedScoped(t, c, "aud-zone", bearer)
			rec := &recordingTripper{}
			tr := &scopeTransport{base: rec, c: c, origin: originOf(tc.base), audience: tc.pinned}
			req, _ := http.NewRequest(http.MethodGet, tc.target, nil)
			if tc.marker != "" {
				req.Header.Set(scopedAudienceHeader, tc.marker)
			}
			resp, err := tr.RoundTrip(req)
			if err != nil {
				t.Fatal(err)
			}
			resp.Body.Close()
			if len(rec.seen) != 1 {
				t.Fatalf("want one request, got %q", rec.seen)
			}
			got := rec.seen[0]
			if strings.Contains(got, "marker=aud") {
				t.Fatalf("audience marker header left the process: %s", got)
			}
			if sent := strings.Contains(got, "auth="+bearer); sent != tc.wantBearer {
				t.Fatalf("scoped bearer sent=%v, want %v: %s", sent, tc.wantBearer, got)
			}
		})
	}
}

// The write-replay origin check in CheckRedirect uses the same comparator.
func TestSameOriginComparesHostsLikeTheNetwork(t *testing.T) {
	u := func(s string) *url.URL {
		v, err := url.Parse(s)
		if err != nil {
			t.Fatal(err)
		}
		return v
	}
	for _, tc := range []struct {
		a, b string
		want bool
	}{
		{"https://[fe80::1%25lan0]/a", "https://[fe80::1%25LAN0]/b", false},
		{"https://k.example/a", "https://K.example/b", false},
		{"https://h.example/a", "http://h.example/b", false},
		{"https://h.example/a", "https://h.example:8443/b", false},
		{"https://[fe80::1%25lan0]/a", "https://[fe80::1%25lan0]/b", true},
		{"https://API.example/a", "https://api.example/b", true},
		{"https://h.example/a", "https://h.example:443/b", true},
	} {
		if got := sameOrigin(u(tc.a), u(tc.b)); got != tc.want {
			t.Errorf("sameOrigin(%s, %s) = %v, want %v", tc.a, tc.b, got, tc.want)
		}
	}
}

// Codex round 1 P2 (fixed, one line): the audience-scoped mint applies the same
// plaintext refusal as every other token request, so the Access ID and API
// password never travel as HTTP Basic over non-loopback http://.
func TestScopedBearerRefusesPlaintextTokenURL(t *testing.T) {
	for _, tc := range []struct {
		tokenURL string
		wantSent bool
	}{
		{"http://api.usa.cloud.watchguard.com/oauth/token", false},
		{"https://api.usa.cloud.watchguard.com/oauth/token", true}, // control: the guard is what stops it
	} {
		rec := &recordingTripper{}
		c := &Client{HTTPClient: &http.Client{Transport: rec}, Config: &config.Config{ClientID: "AID-plain", ClientSecret: "PW-plain", TokenURL: tc.tokenURL}}
		_, _, err := c.ScopedBearer(t.Context(), "aud-plain-"+tc.tokenURL)
		if sent := len(rec.seen) > 0; sent != tc.wantSent {
			t.Fatalf("%s: token request sent=%v, want %v (err=%v)", tc.tokenURL, sent, tc.wantSent, err)
		}
		if !tc.wantSent && err == nil {
			t.Fatalf("%s: plaintext mint returned no error", tc.tokenURL)
		}
	}
}
