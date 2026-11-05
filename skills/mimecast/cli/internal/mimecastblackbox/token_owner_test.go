// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

// Hand-written (NOT generated; handfixes.json mimecast-token-bound-to-client).

package mimecastblackbox

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

// Codex round-2 failing input: two Mimecast API apps, one per customer, in one
// HOME. Each app's run must send the token minted for that app, whatever ran
// before it: a token sent under the other app's client acts on the other
// customer's tenant.
func TestStoredTokenNeverServesAnotherClient(t *testing.T) {
	const (
		appA, secretA = "cid-" + "AppA4Kq9Lm2Xw7", "csec-" + "AppA8Rb3Vn6Qs1"
		appB, secretB = "cid-" + "AppB5Hd2Pz8Ty3", "csec-" + "AppB1Wc7Ze4Ha9"
	)
	tokens := map[string]string{appA: "tok-" + "AppA6Jp3Nq8Lm2", appB: "tok-" + "AppB2Gx9Kr4Ly7"}
	var mu sync.Mutex
	var bearers []string
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/oauth/token" {
			user, _, _ := r.BasicAuth()
			if user == "" {
				_ = r.ParseForm()
				user = r.Form.Get("client_id")
			}
			_, _ = io.WriteString(w, `{"access_token":"`+tokens[user]+`","token_type":"Bearer","expires_in":1800}`)
			return
		}
		mu.Lock()
		bearers = append(bearers, strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer "))
		mu.Unlock()
		_, _ = io.WriteString(w, `{"id":"jx-ok","description":"Plain journal"}`)
	}))
	t.Cleanup(srv.Close)
	urls := []string{"MIMECAST_BASE_URL=" + srv.URL, "MIMECAST_TOKEN_URL=" + srv.URL + "/oauth/token"}
	as := func(home, id, secret string) []string {
		e := append(baseEnv(home), urls...)
		if id != "" {
			e = append(e, "MIMECAST_CLIENT_ID="+id, "MIMECAST_CLIENT_SECRET="+secret)
		}
		return e
	}
	get := func(t *testing.T, environ []string, wantToken, who string) {
		t.Helper()
		mu.Lock()
		before := len(bearers)
		mu.Unlock()
		if out, code := runWith(t, environ, "", "journaling", "get", "jx-ok", "--json"); code != 0 {
			t.Fatalf("%s: exit %d:\n%s", who, code, out)
		}
		mu.Lock()
		defer mu.Unlock()
		if len(bearers) == before {
			t.Fatalf("%s: no API request", who)
		}
		if got := bearers[len(bearers)-1]; got != wantToken {
			t.Fatalf("%s sent another app's token (want %s's)", who, map[string]string{tokens[appA]: "app A", tokens[appB]: "app B"}[wantToken])
		}
	}

	t.Run("login A, env B, then A", func(t *testing.T) {
		home := t.TempDir()
		if out, code := runWith(t, as(home, "", ""), "", "auth", "login", "--client-id", appA, "--client-secret", secretA); code != 0 {
			t.Fatalf("auth login: exit %d:\n%s", code, out)
		}
		get(t, as(home, appB, secretB), tokens[appB], "app B from the environment")
		get(t, as(home, "", ""), tokens[appA], "app A from its saved login")
	})
	t.Run("env B, then env A", func(t *testing.T) {
		home := t.TempDir()
		get(t, as(home, appB, secretB), tokens[appB], "app B from the environment")
		get(t, as(home, appA, secretA), tokens[appA], "app A from the environment")
	})
}
