// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package client

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"apple-business-pp-cli/internal/cliutil"
	"apple-business-pp-cli/internal/cliutil/testenv"
	"apple-business-pp-cli/internal/config"
	"apple-business-pp-cli/internal/credkit"
)

// withheldFirst refuses the first refuse requests as the kit does, then answers a token.
// mintedFixture is split so no scanner reads it as a key.
const mintedFixture = "Minted-" + "Lk52Tg83Hs"

type withheldFirst struct {
	refuse     int
	assertions []string
}

func (w *withheldFirst) RoundTrip(r *http.Request) (*http.Response, error) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		return nil, err
	}
	form, err := url.ParseQuery(string(body))
	if err != nil {
		return nil, err
	}
	w.assertions = append(w.assertions, form.Get("client_assertion"))
	if len(w.assertions) <= w.refuse {
		return nil, credkit.ErrWithheld
	}
	return &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": {"application/json"}},
		Body: io.NopCloser(strings.NewReader(`{"access_token":"` + mintedFixture + `","expires_in":3600}`)), Request: r}, nil
}

// About one signature in a thousand runs the kit's check out of budget, and the request is
// withheld (.review/kit-findings.md). A new signature is a new request; the kit judges it
// again, so what it would really withhold is withheld every time.
func TestMintSignsAgainAfterAWithheldRequest(t *testing.T) {
	testenv.Isolate(t, cliutil.ConfigDir, cliutil.DataDir, cliutil.StateDir, cliutil.CacheDir)
	mint := func(t *testing.T, refuse int) (*withheldFirst, error) {
		t.Helper()
		cfg := &config.Config{
			BaseURL: "https://api-business.apple.com/v1", TokenURL: "https://account.apple.com/auth/oauth2/token",
			ClientID: "BUSINESSAPI.retry", ClientSecret: appleTestKeyPEM(t), AppleBusinessKeyId: "kid-1",
		}
		c := New(cfg, 10*time.Second, 0)
		rt := &withheldFirst{refuse: refuse}
		c.HTTPClient = &http.Client{Transport: rt}
		return rt, c.mintClientCredentials(context.Background(), cfg.ClientID, cfg.ClientSecret)
	}
	t.Run("once", func(t *testing.T) {
		rt, err := mint(t, 1)
		if err != nil {
			t.Fatal(err)
		}
		if len(rt.assertions) != 2 || rt.assertions[0] == rt.assertions[1] {
			t.Errorf("want two requests with different signatures, got %d", len(rt.assertions))
		}
	})
	t.Run("always", func(t *testing.T) {
		rt, err := mint(t, 100)
		if !errors.Is(err, credkit.ErrWithheld) {
			t.Errorf("err = %v", err)
		}
		if len(rt.assertions) != 2 {
			t.Errorf("want two requests, got %d", len(rt.assertions))
		}
	})
}
