// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package credwire_test

// entra-token-form-auth (issue #691). An Entra token's payload carries the
// client ID in base64url. Sent as HTTP Basic, the next token request carries
// that same base64 when the claim falls at the Basic header's alignment, so
// the kit withheld it as a piece of the held token and the process could
// never renew. The kit only judges a request to an allowed Entra host, which
// no loopback fake can be, so this drives the real kit and the real client in
// a child process with a stub under the kit's transport: no network.

import (
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"nerdio-pp-cli/internal/client"
	"nerdio-pp-cli/internal/config"
	"nerdio-pp-cli/internal/credkit"
	"nerdio-pp-cli/internal/credwire"
)

const (
	renewalChildEnv = "NERDIO_TEST_RENEWAL_CHILD"
	renewalAppID    = "3f2a9c1e-7b4d-4e6a-9c1f-5a7b9d1e3f2b"
	renewalPass     = "fixture~renewal-pass-not-a-secret"
	renewalTenant   = "0e1f2a3b-4c5d-4e6f-8a9b-0c1d2e3f4a5b"
)

// entraToken is an Entra-shaped token whose appid claim starts at a multiple
// of three bytes into the payload: base64 of the bare client ID, whole.
func entraToken(n int) string {
	head := `{"aud":"https://nmm.contoso.test","iss":"https://sts.windows.net/` + renewalTenant + `/","aio":"`
	for (len(head)+len(`","appid":"`))%3 != 0 {
		head += "A"
	}
	payload := head + `","appid":"` + renewalAppID + fmt.Sprintf(`","uti":"renewal-%d"}`, n)
	enc := base64.RawURLEncoding
	return enc.EncodeToString([]byte(`{"alg":"RS256","typ":"JWT"}`)) + "." + enc.EncodeToString([]byte(payload)) + ".c2lnbmF0dXJlLWZpeHR1cmU"
}

type renewalStub struct{ mints, reads int }

func (s *renewalStub) RoundTrip(r *http.Request) (*http.Response, error) {
	answer := func(code int, body string) (*http.Response, error) {
		return &http.Response{StatusCode: code, Header: http.Header{"Content-Type": {"application/json"}}, Body: io.NopCloser(strings.NewReader(body)), Request: r}, nil
	}
	if r.URL.Host != "login.microsoftonline.com" {
		s.reads++
		return answer(200, `[]`)
	}
	raw, _ := io.ReadAll(r.Body)
	form, err := url.ParseQuery(string(raw))
	// Either client authentication is accepted, as Entra does: on the old HTTP
	// Basic code the first mint answers and the second is what the kit withholds.
	id, secret, basic := r.BasicAuth()
	if !basic {
		id, secret = form.Get("client_id"), form.Get("client_secret")
	}
	if err != nil || id != renewalAppID || secret != renewalPass {
		return answer(401, `{"error":"invalid_client"}`)
	}
	s.mints++
	// expires_in under the client's 60s window: the next call must mint again.
	return answer(200, fmt.Sprintf(`{"access_token":%q,"expires_in":1,"token_type":"Bearer"}`, entraToken(s.mints)))
}

func renewalChild() {
	credkit.Start(credwire.Declarations)
	credwire.AfterStart()
	stub := &renewalStub{}
	client.OutboundGuard = func(http.RoundTripper) http.RoundTripper { return credkit.Transport(stub) }
	cfg, err := config.Load("")
	if err != nil {
		credkit.Exit(3)
	}
	c := client.New(cfg, 5*time.Second, 0)
	for _, path := range []string{"/rest-api/v1/accounts", "/rest-api/v1/accounts/1"} {
		if _, err := c.GetNoCache(context.Background(), path, nil); err != nil {
			fmt.Fprintln(os.Stderr, err)
			credkit.Exit(4)
		}
	}
	if stub.mints != 2 || stub.reads != 2 {
		fmt.Fprintf(os.Stderr, "mints %d reads %d, want 2 and 2\n", stub.mints, stub.reads)
		credkit.Exit(5)
	}
	credkit.Exit(0)
}

func TestRenewalMintIsNotWithheld(t *testing.T) {
	if os.Getenv(renewalChildEnv) == "1" {
		renewalChild()
		return
	}
	// The fixture must be the shape that collides, or this test proves nothing.
	basic := base64.StdEncoding.EncodeToString([]byte(renewalAppID + ":" + renewalPass))
	shared := base64.StdEncoding.EncodeToString([]byte(renewalAppID))
	if !strings.HasPrefix(basic, shared) || !strings.Contains(entraToken(1), shared) {
		t.Fatalf("the fixture token does not carry the Basic header's base64 of the client ID")
	}
	home := t.TempDir()
	cmd := exec.Command(os.Args[0], "-test.run=^TestRenewalMintIsNotWithheld$")
	cmd.Env = sandboxEnv(home, renewalChildEnv+"=1", "NERDIO_NO_CONFIG_WRITE=1",
		"NERDIO_BASE_URL=https://nmm.contoso.test",
		"NERDIO_TOKEN_URL=https://login.microsoftonline.com/"+renewalTenant+"/oauth2/v2.0/token",
		"NERDIO_OAUTH_SCOPE="+renewalAppID+"/.default",
		"NERDIO_CLIENT_ID="+renewalAppID, "NERDIO_CLIENT_SECRET="+renewalPass)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("the renewal mint failed: %v\n%s", err, out)
	}
}
