// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package client

// nerdio handfixes token-url-from-operator-only and oauth-scope-required, on
// the automatic mint (the internal/cli tests drive `auth login` and a CLI
// command end to end): with NERDIO_TOKEN_URL unset or still naming the
// placeholder tenant, or NERDIO_OAUTH_SCOPE unset, the client posts nothing
// to any host (the NMM instance, the Entra tenant, the placeholder) and the
// error names the variable to set. With both set, the token request asks for
// exactly NERDIO_OAUTH_SCOPE.

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"nerdio-pp-cli/internal/config"
)

func TestMintWithoutTokenURLOrScopeSendsNothing(t *testing.T) {
	for _, c := range []struct {
		name, tokenURL, scope, names string
		want                         error
	}{
		{"NERDIO_TOKEN_URL unset", "", nerdioTestScope, "NERDIO_TOKEN_URL", ErrNoNerdioTokenURL},
		{"NERDIO_TOKEN_URL still the placeholder tenant", "https://login.microsoftonline.com/YOUR_TENANT_ID/oauth2/v2.0/token", nerdioTestScope, "NERDIO_TOKEN_URL", ErrNoNerdioTokenURL},
		{"NERDIO_OAUTH_SCOPE unset", "{entra}", "", "NERDIO_OAUTH_SCOPE", ErrNoNerdioScope},
	} {
		t.Run(c.name, func(t *testing.T) {
			isolateCredentialStore(t)
			nmm, entra := newNMMFake(t), newEntraFake(t, "")
			const id = "noTokenUrlClient-4e1a"
			t.Setenv("NERDIO_BASE_URL", nmm.srv.URL)
			t.Setenv("NERDIO_TOKEN_URL", strings.ReplaceAll(c.tokenURL, "{entra}", entra.tokenURL))
			t.Setenv("NERDIO_OAUTH_SCOPE", c.scope)
			t.Setenv("NERDIO_CLIENT_ID", id)
			t.Setenv("NERDIO_CLIENT_SECRET", "secret-for-"+id)
			t.Setenv("NERDIO_NO_CONFIG_WRITE", "1")
			cfg, err := config.Load("")
			if err != nil {
				t.Fatal(err)
			}
			cl := New(cfg, 5*time.Second, 0)
			cl.HTTPClient.Transport = r1NoNetwork{t} // the placeholder tenant is a real host: nothing may be dialed
			_, err = cl.Get(context.Background(), "/rest-api/v1/accounts", nil)
			if !errors.Is(err, c.want) || !strings.Contains(err.Error(), c.names) {
				t.Fatalf("err = %v, want %v naming %s", err, c.want, c.names)
			}
			if _, requests, _, _ := entra.state(); requests != 0 {
				t.Errorf("the Entra token endpoint received %d request(s)", requests)
			}
			if auths := nmm.sent(); len(auths) != 0 {
				t.Errorf("the NMM instance received %d request(s) (Authorization %v)", len(auths), auths)
			}
		})
	}
}

// The control for the test above, and the scope rule: with both variables set
// the same client mints once at the Entra token URL, asking for exactly
// NERDIO_OAUTH_SCOPE (never api://<client id>/.default), and sends the minted
// token to the NMM instance.
func TestMintAsksForExactlyTheConfiguredScope(t *testing.T) {
	isolateCredentialStore(t)
	nmm, entra := newNMMFake(t), newEntraFake(t, "")
	const id, scope = "scopeClient-8c2f", "  9b8a7c6d-5e4f-4a3b-8c2d-1e0f9a8b7c6d/.default "
	t.Setenv("NERDIO_BASE_URL", nmm.srv.URL)
	t.Setenv("NERDIO_TOKEN_URL", entra.tokenURL)
	t.Setenv("NERDIO_OAUTH_SCOPE", scope)
	t.Setenv("NERDIO_CLIENT_ID", id)
	t.Setenv("NERDIO_CLIENT_SECRET", "secret-for-"+id)
	t.Setenv("NERDIO_NO_CONFIG_WRITE", "1")
	cfg, err := config.Load("")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := New(cfg, 5*time.Second, 0).Get(context.Background(), "/rest-api/v1/accounts", nil); err != nil {
		t.Fatal(err)
	}
	mints, requests, scopes, refused := entra.state()
	if mints[id] != 1 || requests != 1 || len(refused) != 0 {
		t.Fatalf("want one accepted mint, got %v in %d request(s), refused %v", mints, requests, refused)
	}
	if scopes[0] != strings.TrimSpace(scope) {
		t.Errorf("the token request asked for scope %q, want exactly NERDIO_OAUTH_SCOPE %q", scopes[0], strings.TrimSpace(scope))
	}
	if auths := nmm.sent(); len(auths) != 1 || auths[0] != "Bearer minted-"+id+"-1" {
		t.Errorf("the NMM instance received %v, want the one minted token", auths)
	}
}
