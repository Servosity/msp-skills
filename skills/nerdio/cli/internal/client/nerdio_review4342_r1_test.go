// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package client

import (
	"context"
	"encoding/base64"
	"os"
	"strings"
	"testing"
	"time"

	"nerdio-pp-cli/internal/cliutil"
	"nerdio-pp-cli/internal/config"
)

// Codex review r1 (4.34.2 reprint), class token bound to another client pair:
// `auth login --client-id A --client-secret A` stores pair A and A's token;
// `auth login` run again with pair B in the environment minted B's token and
// SaveTokens wrote it beside A's stored pair (the environment pair itself
// stays off disk), so the next run with no environment pair sent B's token
// while A's pair was active. The refresh grant's save had the same shape.
// Every token save now passes one gate (config.SaveTokens): a token is saved
// only beside the pair it was minted for, so B's save is refused, and the
// stored token stays A's.
func TestTokenIsSavedOnlyBesideThePairItWasMintedFor(t *testing.T) {
	isolateCredentialStore(t)
	nmm, entra := newNMMFake(t), newEntraFake(t, "")
	t.Setenv("NERDIO_BASE_URL", nmm.srv.URL)
	t.Setenv("NERDIO_TOKEN_URL", entra.tokenURL)
	const idA, idB = "save-client-a-4c2e", "save-client-b-9f1d"
	const tokenA, tokenB = "stored-token-for-a-71c3", "env-token-for-b-52e8"
	files, err := config.Load("")
	if err != nil {
		t.Fatal(err)
	}
	files.MarkCredentialsExplicit(true, true)
	if err := files.SaveTokens(idA, "secret-for-"+idA, tokenA, "", time.Now().Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	// `auth login` with pair B from the environment (no --client-id flags).
	t.Setenv("NERDIO_CLIENT_ID", idB)
	t.Setenv("NERDIO_CLIENT_SECRET", "secret-for-"+idB)
	envCfg, err := config.Load("")
	if err != nil {
		t.Fatal(err)
	}
	envCfg.MarkCredentialsExplicit(false, false)
	saveErr := envCfg.SaveTokens(idB, "secret-for-"+idB, tokenB, "", time.Now().Add(time.Hour))
	// Back to the stored pair alone.
	t.Setenv("NERDIO_CLIENT_ID", "")
	t.Setenv("NERDIO_CLIENT_SECRET", "")
	cfg, err := config.Load("")
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(cfg.ClientID) != idA {
		t.Fatalf("the stored pair is %q, want %q; the test did not set up pair A", cfg.ClientID, idA)
	}
	if _, err := New(cfg, 5*time.Second, 0).Get(context.Background(), "/rest-api/v1/accounts", nil); err != nil {
		t.Fatal(err)
	}
	for _, a := range nmm.sent() {
		if strings.Contains(a, tokenB) {
			t.Errorf("client A's request carried %q: the token minted for pair B was saved beside pair A and sent with it", a)
		}
	}
	credPath, err := cliutil.CredentialsFilePath()
	if err != nil {
		t.Fatal(err)
	}
	if b, err := os.ReadFile(credPath); err == nil && strings.Contains(string(b), tokenB) {
		t.Errorf("the credential store holds pair B's token beside pair A:\n%s", b)
	}
	if saveErr == nil || !strings.Contains(saveErr.Error(), "NERDIO_CLIENT_ID") {
		t.Errorf("SaveTokens for an environment pair the files do not hold returned %v, want a refusal naming NERDIO_CLIENT_ID", saveErr)
	}
}

// The stored token's own claims are a second signal beside the files: an
// Entra access token names the client it was issued to (appid, v1; azp, v2).
// A credential store that already holds another client's token beside pair A
// (written by a release before the save gate) is not sent for A; A mints its
// own.
func TestStoredTokenIssuedToAnotherClientIsNotSent(t *testing.T) {
	isolateCredentialStore(t)
	nmm, entra := newNMMFake(t), newEntraFake(t, "")
	t.Setenv("NERDIO_BASE_URL", nmm.srv.URL)
	t.Setenv("NERDIO_TOKEN_URL", entra.tokenURL)
	const idA, idB = "claim-client-a-2d7e", "claim-client-b-6a1f"
	seg := func(s string) string { return base64.RawURLEncoding.EncodeToString([]byte(s)) }
	for _, claim := range []string{"appid", "azp"} {
		t.Run(claim, func(t *testing.T) {
			jwtB := seg(`{"alg":"RS256","typ":"JWT"}`) + "." + seg(`{"aud":"nmm","`+claim+`":"`+idB+`","exp":4102444800}`) + "." + seg("signature-bytes-"+claim)
			files, err := config.Load("")
			if err != nil {
				t.Fatal(err)
			}
			files.MarkCredentialsExplicit(true, true)
			if err := files.SaveTokens(idA, "secret-for-"+idA, jwtB, "", time.Now().Add(time.Hour)); err != nil {
				t.Fatal(err)
			}
			cfg, err := config.Load("")
			if err != nil {
				t.Fatal(err)
			}
			if _, err := New(cfg, 5*time.Second, 0).Get(context.Background(), "/rest-api/v1/accounts", nil); err != nil {
				t.Fatal(err)
			}
			auths := nmm.sent()
			if len(auths) == 0 {
				t.Fatal("no API request was sent")
			}
			if last := auths[len(auths)-1]; strings.Contains(last, jwtB) || !strings.HasPrefix(last, "Bearer minted-"+idA+"-") {
				t.Errorf("client A's request carried %q: want a token minted for A, never the stored token issued to %s", last, idB)
			}
			if mints, _, _, _ := entra.state(); mints[idA] == 0 {
				t.Errorf("no token was minted for A: %v", mints)
			}
		})
	}
	// Control: a stored JWT issued to the stored client itself (any case) is
	// sent as stored, with no mint (its own client ID: the subtests left a
	// process token for A).
	const idC = "claim-client-c-9b4a"
	jwtC := seg(`{"alg":"RS256","typ":"JWT"}`) + "." + seg(`{"aud":"nmm","appid":"`+strings.ToUpper(idC)+`","azp":"`+idC+`","exp":4102444800}`) + "." + seg("signature-bytes-c")
	files, err := config.Load("")
	if err != nil {
		t.Fatal(err)
	}
	files.MarkCredentialsExplicit(true, true)
	if err := files.SaveTokens(idC, "secret-for-"+idC, jwtC, "", time.Now().Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	cfg, err := config.Load("")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := New(cfg, 5*time.Second, 0).Get(context.Background(), "/rest-api/v1/accounts", nil); err != nil {
		t.Fatal(err)
	}
	auths := nmm.sent()
	if mints, _, _, _ := entra.state(); auths[len(auths)-1] != "Bearer "+jwtC || mints[idC] != 0 {
		t.Errorf("control: C's own stored token must be sent without a mint; sent %q, mints %v", auths[len(auths)-1], mints)
	}
}
