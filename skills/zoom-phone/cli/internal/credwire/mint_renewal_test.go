// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package credwire_test

// zoom-token-form-auth (issue #698's class, nerdio #691). A Zoom
// Server-to-Server token's payload carries the client ID ("iss":"zm:cid:<id>";
// a real token was seen with it at a multiple of three bytes). Sent as HTTP
// Basic to zoom.us, a token request carries the same base64 run, and the token
// is held for the API base only, so the kit withheld the first mint after a
// restart that loaded an expired token (and, without the header:authorization
// declaration, every renewal). The kit only judges a request to the real token
// host, which no loopback fake can be, so this drives the real kit and the
// real client in a child process with a stub under the kit's transport, at
// each of the three base64 alignments of the client ID inside the token, for
// a renewal in one process and for a restart: no network. The fixture token
// shares no text with the fixture secret, so only the alignment can collide.

import (
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"

	"zoom-phone-pp-cli/internal/client"
	"zoom-phone-pp-cli/internal/cliutil"
	"zoom-phone-pp-cli/internal/config"
	"zoom-phone-pp-cli/internal/credkit"
	"zoom-phone-pp-cli/internal/credwire"
)

const (
	renewalChildEnv = "ZOOM_PHONE_TEST_RENEWAL_CHILD"
	renewalAlignEnv = "ZOOM_PHONE_TEST_RENEWAL_ALIGN"
	renewalClientID = "zoomclientidexample001"
	renewalPass     = "fixture~renewal-pass-not-a-secret"
	renewalAccount  = "Ab1Cd2Ef3Gh4Ij5Kl6Mn7O"
)

// zoomToken is a Zoom-shaped token whose client ID starts align bytes past a
// multiple of three into the payload; align 0 carries the base64 of the bare
// client ID whole.
func zoomToken(n, align int) string {
	head := `{"aud":"https://oauth.zoom.us","uid":"u1","ver":10,"auid":"`
	for (len(head)+len(`","iss":"zm:cid:`))%3 != align {
		head += "A"
	}
	payload := head + `","iss":"zm:cid:` + renewalClientID + fmt.Sprintf(`","aid":"%s","n":%d}`, renewalAccount, n)
	enc := base64.RawURLEncoding
	return enc.EncodeToString([]byte(`{"sv":"000001","alg":"HS512","v":"2.0"}`)) + "." + enc.EncodeToString([]byte(payload)) + ".c2lnLWJ5dGVzLTAxMjM"
}

type renewalStub struct {
	align        int
	mints, reads int
	order        []string
}

func (s *renewalStub) RoundTrip(r *http.Request) (*http.Response, error) {
	answer := func(code int, body string) (*http.Response, error) {
		return &http.Response{StatusCode: code, Header: http.Header{"Content-Type": {"application/json"}}, Body: io.NopCloser(strings.NewReader(body)), Request: r}, nil
	}
	if r.URL.Host != "zoom.us" {
		s.reads++
		return answer(200, `{"users":[]}`)
	}
	raw, _ := io.ReadAll(r.Body)
	form, err := url.ParseQuery(string(raw))
	id, secret, basic := r.BasicAuth()
	if !basic {
		id, secret = form.Get("client_id"), form.Get("client_secret")
		key, _, _ := strings.Cut(string(raw), "=")
		s.order = append(s.order, key)
	}
	if err != nil || id != renewalClientID || secret != renewalPass || form.Get("account_id") != renewalAccount {
		return answer(401, `{"reason":"Invalid client_id or client_secret","error":"invalid_client"}`)
	}
	s.mints++
	// expires_in under the client's 60s window: the next call must mint again.
	return answer(200, fmt.Sprintf(`{"access_token":%q,"expires_in":1,"token_type":"bearer"}`, zoomToken(s.mints, s.align)))
}

func renewalChild() {
	credkit.Start(credwire.Declarations)
	align, _ := strconv.Atoi(os.Getenv(renewalAlignEnv))
	stub := &renewalStub{align: align}
	cliutil.OutboundGuard = func(http.RoundTripper) http.RoundTripper { return credkit.Transport(stub) }
	cfg, err := config.Load("")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		credkit.Exit(3)
	}
	c := client.New(cfg, 5*time.Second, 0)
	for _, path := range []string{"/phone/users", "/phone/users/1"} {
		if _, err := c.GetNoCache(context.Background(), path, nil); err != nil {
			fmt.Fprintf(os.Stderr, "GET %s after %d mints: %v\n", path, stub.mints, err)
			credkit.Exit(4)
		}
	}
	if stub.mints != 2 || stub.reads != 2 {
		fmt.Fprintf(os.Stderr, "mints %d reads %d, want 2 and 2\n", stub.mints, stub.reads)
		credkit.Exit(5)
	}
	// A form body that starts with a credential field would hold its tail,
	// grant_type included (cliutil.TokenForm).
	for _, first := range stub.order {
		if first != "grant_type" {
			fmt.Fprintf(os.Stderr, "token form starts with %q, want grant_type\n", first)
			credkit.Exit(6)
		}
	}
	credkit.Exit(0)
}

func renewalEnv(home string, extra ...string) []string {
	env := []string{
		"HTTPS_PROXY=http://127.0.0.1:9", "HTTP_PROXY=http://127.0.0.1:9",
		"PATH=" + os.Getenv("PATH"), "HOME=" + home, "USERPROFILE=" + home, "ZOOM_PHONE_HOME=" + home,
		"XDG_CONFIG_HOME=" + filepath.Join(home, "config"), "XDG_DATA_HOME=" + filepath.Join(home, "data"),
		"XDG_STATE_HOME=" + filepath.Join(home, "state"), "XDG_CACHE_HOME=" + filepath.Join(home, "cache"),
		"TMPDIR=" + os.TempDir(), "NO_COLOR=1",
	}
	if runtime.GOOS == "windows" {
		for _, k := range []string{"SYSTEMROOT", "TEMP", "TMP", "APPDATA", "LOCALAPPDATA"} {
			if v := os.Getenv(k); v != "" {
				env = append(env, k+"="+v)
			}
		}
	}
	return append(env, extra...)
}

func TestRenewalMintIsNotWithheld(t *testing.T) {
	if os.Getenv(renewalChildEnv) == "1" {
		renewalChild()
		return
	}
	// Align 0 must be the shape that collides, or it proves nothing.
	basic := base64.StdEncoding.EncodeToString([]byte(renewalClientID + ":" + renewalPass))
	shared := base64.StdEncoding.EncodeToString([]byte(renewalClientID))[:28]
	if !strings.HasPrefix(basic, shared) || !strings.Contains(zoomToken(1, 0), shared) {
		t.Fatalf("the fixture token does not carry the Basic header's base64 of the client ID")
	}
	for _, restart := range []bool{false, true} {
		for align := 0; align < 3; align++ {
			t.Run(fmt.Sprintf("restart=%v/align%d", restart, align), func(t *testing.T) {
				home := t.TempDir()
				cfgPath := filepath.Join(home, "config.toml")
				if restart {
					expired := "access_token = \"" + zoomToken(0, align) + "\"\ntoken_expiry = 2000-01-01T00:00:00Z\n"
					if err := os.WriteFile(cfgPath, []byte(expired), 0o600); err != nil {
						t.Fatal(err)
					}
				}
				cmd := exec.Command(os.Args[0], "-test.run=^TestRenewalMintIsNotWithheld$")
				cmd.Env = renewalEnv(home, renewalChildEnv+"=1", renewalAlignEnv+"="+strconv.Itoa(align),
					"ZOOM_PHONE_TOKEN_URL=https://zoom.us/oauth/token", "ZOOM_PHONE_CONFIG="+cfgPath,
					"ZOOM_PHONE_ACCOUNT_ID="+renewalAccount,
					"ZOOM_PHONE_CLIENT_ID="+renewalClientID, "ZOOM_PHONE_CLIENT_SECRET="+renewalPass)
				if out, err := cmd.CombinedOutput(); err != nil {
					t.Fatalf("the renewal mint failed: %v\n%s", err, out)
				}
			})
		}
	}
}
