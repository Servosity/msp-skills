// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package client

import (
	"context"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"nerdio-pp-cli/internal/cliutil"
	"nerdio-pp-cli/internal/config"
)

// minted-token-bound-to-its-client: the files hold client pair A and the token
// minted for A (what `auth login` with A's pair writes); the environment
// supplies pair B. The API request carries a token freshly minted for B at
// the Entra token URL, never A's; the credential store still holds A's token,
// and B's token is written to no file; a second request in the same process
// (a second client, as the MCP server builds one per call) reuses B's token
// without minting again.
func TestEnvironmentPairMintsItsOwnTokenAndKeepsTheStoredOne(t *testing.T) {
	home := isolateCredentialStore(t)
	nmm, entra := newNMMFake(t), newEntraFake(t, "")
	t.Setenv("NERDIO_BASE_URL", nmm.srv.URL)
	t.Setenv("NERDIO_TOKEN_URL", entra.tokenURL)
	const idA, idB = "bind-client-a-5e1c", "bind-client-b-8d4f"
	const tokenA = "stored-token-for-client-a-2b9e"
	files, err := config.Load("")
	if err != nil {
		t.Fatal(err)
	}
	if err := files.SaveTokens(idA, "secret-for-"+idA, tokenA, "", time.Now().Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	credPath, err := cliutil.CredentialsFilePath()
	if err != nil {
		t.Fatal(err)
	}
	if b, err := os.ReadFile(credPath); err != nil || !strings.Contains(string(b), tokenA) || !strings.Contains(string(b), idA) {
		t.Fatalf("the files do not hold pair A and token A; the test did not run (err %v):\n%s", err, b)
	}
	// Control: with no environment pair, A's stored token is A's and is sent
	// as stored, with no mint.
	send := func() {
		t.Helper()
		cfg, err := config.Load("")
		if err != nil {
			t.Fatal(err)
		}
		if _, err := New(cfg, 5*time.Second, 0).Get(context.Background(), "/rest-api/v1/accounts", nil); err != nil {
			t.Fatal(err)
		}
	}
	send()
	if mints, requests, _, _ := entra.state(); requests != 0 || len(mints) != 0 {
		t.Fatalf("control: A's own stored token must be sent without a mint; token requests %d, mints %v", requests, mints)
	}
	if auths := nmm.sent(); len(auths) != 1 || auths[0] != "Bearer "+tokenA {
		t.Fatalf("control: want A's stored token on the one API request, sent %v", auths)
	}
	t.Setenv("NERDIO_CLIENT_ID", idB)
	t.Setenv("NERDIO_CLIENT_SECRET", "secret-for-"+idB)
	send()
	send()
	mints, requests, scopes, refused := entra.state()
	if mints[idB] != 1 || mints[idA] != 0 || requests != 1 || len(refused) != 0 {
		t.Fatalf("want exactly one accepted mint, for B, across two requests in one process; got %v in %d request(s), refused %v", mints, requests, refused)
	}
	if scopes[0] != nerdioTestScope {
		t.Errorf("the mint asked for scope %q, want NERDIO_OAUTH_SCOPE %q", scopes[0], nerdioTestScope)
	}
	auths := nmm.sent()
	if len(auths) != 3 {
		t.Fatalf("want three API requests, got %v", auths)
	}
	for _, a := range auths[1:] {
		if a != "Bearer minted-"+idB+"-1" {
			t.Errorf("a request for client B carried %q, want B's minted token (A's stored token authenticates as A)", a)
		}
	}
	b, err := os.ReadFile(credPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), tokenA) || !strings.Contains(string(b), idA) {
		t.Errorf("the credential store lost pair A or token A:\n%s", b)
	}
	_ = filepath.WalkDir(home, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		data, _ := os.ReadFile(path)
		for _, v := range []string{"minted-" + idB, "secret-for-" + idB, idB} {
			if strings.Contains(string(data), v) {
				t.Errorf("%s holds %q: an environment pair's credential or its token was persisted", path, v)
			}
		}
		return nil
	})
}

// A token minted for an environment pair is bound to that pair and the token
// URL it was minted at: the same pair against another Entra tenant (another
// NERDIO_TOKEN_URL, for another NMM instance) mints its own token there, and
// coming back to the first tenant reuses the token minted for it.
func TestProcessTokenIsBoundToItsTokenURL(t *testing.T) {
	isolateCredentialStore(t)
	type instance struct {
		nmm   *nmmFake
		entra *entraFake
	}
	one := instance{newNMMFake(t), newEntraFake(t, "one-")}
	two := instance{newNMMFake(t), newEntraFake(t, "two-")}
	const id = "bind-client-c-3a7d"
	t.Setenv("NERDIO_CLIENT_ID", id)
	t.Setenv("NERDIO_CLIENT_SECRET", "secret-for-"+id)
	t.Setenv("NERDIO_NO_CONFIG_WRITE", "1")
	for _, in := range []instance{one, two, one} {
		t.Setenv("NERDIO_BASE_URL", in.nmm.srv.URL)
		t.Setenv("NERDIO_TOKEN_URL", in.entra.tokenURL)
		cfg, err := config.Load("")
		if err != nil {
			t.Fatal(err)
		}
		if _, err := New(cfg, 5*time.Second, 0).Get(context.Background(), "/rest-api/v1/accounts", nil); err != nil {
			t.Fatal(err)
		}
	}
	for tag, in := range map[string]instance{"one-": one, "two-": two} {
		mints, requests, _, refused := in.entra.state()
		if mints[id] != 1 || requests != 1 || len(refused) != 0 {
			t.Errorf("tenant %s: want one accepted mint for its own token, got %v in %d request(s), refused %v", tag, mints, requests, refused)
		}
		auths := in.nmm.sent()
		if len(auths) == 0 {
			t.Errorf("NMM instance %s received no request", tag)
		}
		for _, a := range auths {
			if a != "Bearer minted-"+tag+id+"-1" {
				t.Errorf("NMM instance %s received %q, want the one token minted at its tenant in this process", tag, a)
			}
		}
	}
}
