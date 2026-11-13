// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package credwire_test

// entra-token-form-auth (issue #698, the same class as nerdio's #691). An Entra
// token's payload carries the client ID in base64url. Sent as HTTP Basic, the
// next token request carries that same base64 when the claim falls at the Basic
// header's alignment, so the kit withheld it as a piece of the held token (which
// may go only to the ImmyBot instance) and the process could never renew. The
// kit only judges a request to an allowed Entra host, which no loopback fake can
// be, so this drives the real kit and the real client in a child process with a
// stub under the kit's transport, at each of the three base64 alignments of the
// client ID inside the token, both for a renewal in one process and for the
// first mint after a restart that loaded an expired token: no network. The fixture token shares no text with
// the fixture secret, so only the alignment can collide.

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

	"immybot-pp-cli/internal/client"
	"immybot-pp-cli/internal/cliutil"
	"immybot-pp-cli/internal/config"
	"immybot-pp-cli/internal/credkit"
	"immybot-pp-cli/internal/credwire"
)

const (
	renewalChildEnv = "IMMYBOT_TEST_RENEWAL_CHILD"
	renewalAlignEnv = "IMMYBOT_TEST_RENEWAL_ALIGN"
	renewalAppID    = "3f2a9c1e-7b4d-4e6a-9c1f-5a7b9d1e3f2b"
	renewalPass     = "fixture~renewal-pass-not-a-secret"
	renewalTenant   = "0e1f2a3b-4c5d-4e6f-8a9b-0c1d2e3f4a5b"
	renewalBase     = "https://contoso.immy.bot"
)

// entraToken is an Entra-shaped token whose appid claim starts align bytes
// past a multiple of three into the payload; align 0 carries the base64 of the
// bare client ID whole.
func entraToken(n, align int) string {
	head := `{"aud":"` + renewalBase + `","iss":"https://sts.windows.net/` + renewalTenant + `/","aio":"`
	for (len(head)+len(`","appid":"`))%3 != align {
		head += "A"
	}
	payload := head + `","appid":"` + renewalAppID + fmt.Sprintf(`","uti":"uti-%d"}`, n)
	enc := base64.RawURLEncoding
	return enc.EncodeToString([]byte(`{"alg":"RS256","typ":"JWT"}`)) + "." + enc.EncodeToString([]byte(payload)) + ".c2lnLWJ5dGVzLTAxMjM"
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
		key, _, _ := strings.Cut(string(raw), "=")
		s.order = append(s.order, key)
	}
	if err != nil || id != renewalAppID || secret != renewalPass {
		return answer(401, `{"error":"invalid_client"}`)
	}
	s.mints++
	// expires_in under the client's 60s window: the next call must mint again.
	return answer(200, fmt.Sprintf(`{"access_token":%q,"expires_in":1,"token_type":"Bearer"}`, entraToken(s.mints, s.align)))
}

func renewalChild() {
	credkit.Start(credwire.Declarations)
	credwire.HoldStartup(os.Args[1:])
	align, _ := strconv.Atoi(os.Getenv(renewalAlignEnv))
	stub := &renewalStub{align: align}
	cliutil.OutboundGuard = func(http.RoundTripper) http.RoundTripper { return credkit.Transport(stub) }
	cfg, err := config.Load("")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		credkit.Exit(3)
	}
	c := client.New(cfg, 5*time.Second, 0)
	for _, path := range []string{"/api/v1/computers", "/api/v1/computers/1"} {
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
		"PATH=" + os.Getenv("PATH"), "HOME=" + home, "USERPROFILE=" + home, "IMMYBOT_HOME=" + home,
		"XDG_CONFIG_HOME=" + filepath.Join(home, "config"), "XDG_DATA_HOME=" + filepath.Join(home, "data"),
		"XDG_STATE_HOME=" + filepath.Join(home, "state"), "XDG_CACHE_HOME=" + filepath.Join(home, "cache"),
		"TMPDIR=" + os.TempDir(), "NO_COLOR=1", "IMMYBOT_NO_LEARN=1",
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
	// Align 0 must be the shape that collides, or this test proves nothing.
	basic := base64.StdEncoding.EncodeToString([]byte(renewalAppID + ":" + renewalPass))
	shared := base64.StdEncoding.EncodeToString([]byte(renewalAppID))
	if !strings.HasPrefix(basic, shared) || !strings.Contains(entraToken(1, 0), shared) {
		t.Fatalf("the fixture token does not carry the Basic header's base64 of the client ID")
	}
	for _, restart := range []bool{false, true} {
		for align := 0; align < 3; align++ {
			t.Run(fmt.Sprintf("restart=%v/align%d", restart, align), func(t *testing.T) {
				home := t.TempDir()
				cfgPath := filepath.Join(home, "config.toml")
				if restart {
					expired := "access_token = \"" + entraToken(0, align) + "\"\ntoken_expiry = 2000-01-01T00:00:00Z\n"
					if err := os.WriteFile(cfgPath, []byte(expired), 0o600); err != nil {
						t.Fatal(err)
					}
				}
				cmd := exec.Command(os.Args[0], "-test.run=^TestRenewalMintIsNotWithheld$")
				cmd.Env = renewalEnv(home, renewalChildEnv+"=1", renewalAlignEnv+"="+strconv.Itoa(align), "IMMYBOT_NO_CONFIG_WRITE=1", "IMMYBOT_CONFIG="+cfgPath,
					"IMMYBOT_BASE_URL="+renewalBase,
					"IMMYBOT_TOKEN_URL=https://login.microsoftonline.com/"+renewalTenant+"/oauth2/v2.0/token",
					"IMMYBOT_OAUTH_SCOPE=api://"+renewalAppID+"/.default",
					"IMMYBOT_CLIENT_ID="+renewalAppID, "IMMYBOT_CLIENT_SECRET="+renewalPass)
				if out, err := cmd.CombinedOutput(); err != nil {
					t.Fatalf("the renewal mint failed: %v\n%s", err, out)
				}
			})
		}
	}
}
