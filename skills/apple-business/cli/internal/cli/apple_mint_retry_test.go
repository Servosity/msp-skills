// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"apple-business-pp-cli/internal/credkit"
)

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

// auth login's mint, as client.TestMintSignsAgainAfterAWithheldRequest for the client's.
func TestLoginMintSignsAgainAfterAWithheldRequest(t *testing.T) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	der, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		t.Fatal(err)
	}
	pemText := string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der}))
	const tokenURL = "https://account.apple.com/auth/oauth2/token"
	once := &withheldFirst{refuse: 1}
	if _, err := mintClientCredentialsToken(&http.Client{Transport: once}, tokenURL, "BUSINESSAPI.retry", pemText, "kid-1"); err != nil {
		t.Fatal(err)
	}
	if len(once.assertions) != 2 || once.assertions[0] == once.assertions[1] {
		t.Errorf("want two requests with different signatures, got %d", len(once.assertions))
	}
	always := &withheldFirst{refuse: 100}
	if _, err := mintClientCredentialsToken(&http.Client{Transport: always}, tokenURL, "BUSINESSAPI.retry", pemText, "kid-1"); !errors.Is(err, credkit.ErrWithheld) {
		t.Errorf("err = %v", err)
	}
	if len(always.assertions) != 2 {
		t.Errorf("want two requests, got %d", len(always.assertions))
	}
}
