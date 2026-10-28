// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

// Teams meeting transcripts are own-tenant only (microsoft-graph handfix:
// transcripts-own-tenant). An MSP reaches its clients' tenants through GDAP and
// multi-tenant apps; reading a client's meetings is never this tool's job. So
// every transcripts command checks the credential before any request:
//
//   - MICROSOFT_GRAPH_HOME_TENANT names the operator's own tenant id, and the
//     token's tid claim must equal it (a client tenant's token is refused);
//   - a user who signed in from another tenant (a GDAP or guest user: the idp
//     claim differs from iss) is refused;
//   - an app-only token (roles, no scp) is refused unless
//     MICROSOFT_GRAPH_TRANSCRIPTS_APP_ONLY=1; Teams also requires an
//     application access policy (Grant-CsApplicationAccessPolicy) for it;
//   - a token whose tenant cannot be read (opaque, or no tid) is refused.
//
// Nothing is kept on disk: the response cache is off for every Graph client
// (msgraph_guard.go), transcripts never touch the local store, and the learn
// journal skips them (isLocalDataInvocation). MCP can switch them off with
// MICROSOFT_GRAPH_MCP_NO_TRANSCRIPTS=1 and can never name an output file.

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strings"

	"microsoft-graph-pp-cli/internal/client"
	"microsoft-graph-pp-cli/internal/cliutil"
)

const (
	homeTenantEnv         = "MICROSOFT_GRAPH_HOME_TENANT"
	transcriptsAppOnlyEnv = "MICROSOFT_GRAPH_TRANSCRIPTS_APP_ONLY"
	mcpNoTranscriptsEnv   = "MICROSOFT_GRAPH_MCP_NO_TRANSCRIPTS"
)

// tokenClaims are the access-token claims the gate reads. Graph verifies the
// signature; the gate only needs which tenant and which kind of token it is.
type tokenClaims struct {
	Tid   string   `json:"tid"`
	Iss   string   `json:"iss"`
	Idp   string   `json:"idp"`
	Scp   string   `json:"scp"`
	Roles []string `json:"roles"`
}

// appOnly reports a client-credentials token: application roles, no delegated scopes.
func (c tokenClaims) appOnly() bool { return c.Scp == "" && len(c.Roles) > 0 }

func decodeTokenClaims(authHeader string) (tokenClaims, bool) {
	tok := strings.TrimSpace(authHeader)
	if len(tok) > 7 && strings.EqualFold(tok[:7], "bearer ") {
		tok = strings.TrimSpace(tok[7:])
	}
	parts := strings.Split(tok, ".")
	if len(parts) != 3 {
		return tokenClaims{}, false
	}
	raw, err := base64.RawURLEncoding.DecodeString(strings.TrimRight(parts[1], "="))
	if err != nil {
		return tokenClaims{}, false
	}
	// Exact claim names only: a struct decode matches keys case-insensitively
	// and the last one wins, so a payload carrying "TID" beside "tid" would be
	// judged by the wrong tenant (adversarial review 2026-10-28).
	var m map[string]json.RawMessage
	if json.Unmarshal(raw, &m) != nil {
		return tokenClaims{}, false
	}
	var c tokenClaims
	for name, dst := range map[string]any{"tid": &c.Tid, "iss": &c.Iss, "idp": &c.Idp, "scp": &c.Scp, "roles": &c.Roles} {
		if v, ok := m[name]; ok && json.Unmarshal(v, dst) != nil {
			return tokenClaims{}, false
		}
	}
	if strings.TrimSpace(c.Tid) == "" {
		return tokenClaims{}, false
	}
	return c, true
}

func transcriptsSwitchedOff() bool {
	return cliutil.IsMCPChildEnv() && os.Getenv(mcpNoTranscriptsEnv) == "1"
}

// transcriptsGate refuses any credential that is not a member's or an
// opted-in app's token for the operator's own tenant. It judges every
// Authorization header the configuration can send (the resolved token and a
// config file's [headers] entry, which replaces it on the wire), then installs
// transcriptsWireCheck so each request is judged again by the header it
// actually carries (Codex r1 class cross-tenant-transcript-read).
func transcriptsGate(c *client.Client) (tokenClaims, error) {
	if transcriptsSwitchedOff() {
		return tokenClaims{}, usageErr(fmt.Errorf("transcripts are switched off for MCP (%s=1)", mcpNoTranscriptsEnv))
	}
	home := strings.TrimSpace(os.Getenv(homeTenantEnv))
	if home == "" {
		return tokenClaims{}, configErr(fmt.Errorf("transcripts are own-tenant only: set %s to your own tenant id (Entra admin center > Overview > Tenant ID); a token for any other tenant is refused", homeTenantEnv))
	}
	if c == nil || c.Config == nil || c.HTTPClient == nil {
		return tokenClaims{}, authErr(errors.New("transcripts need a Microsoft Graph access token; none is configured"))
	}
	headers := []string{c.Config.AuthHeader()}
	for k, v := range c.Config.Headers {
		if strings.EqualFold(strings.TrimSpace(k), "Authorization") {
			headers = append(headers, v)
		}
	}
	var claims tokenClaims
	for _, h := range headers {
		if strings.TrimSpace(h) == "" && len(headers) > 1 {
			continue // the resolved token is empty and a [headers] entry supplies it
		}
		got, err := transcriptsCredentialClaims(h, home)
		if err != nil {
			return tokenClaims{}, err
		}
		claims = got
	}
	next := c.HTTPClient.Transport
	if next == nil {
		next = http.DefaultTransport
	}
	if _, ok := next.(transcriptsWireCheck); !ok {
		c.HTTPClient.Transport = transcriptsWireCheck{home: home, next: next}
	}
	return claims, nil
}

// transcriptsCredentialClaims applies the own-tenant rules to one
// Authorization header value.
func transcriptsCredentialClaims(authHeader, home string) (tokenClaims, error) {
	claims, ok := decodeTokenClaims(authHeader)
	if !ok {
		return tokenClaims{}, authErr(errors.New("transcripts need a Microsoft Entra access token whose tenant can be read; this token is missing or opaque, so it cannot be confirmed as your own tenant's"))
	}
	if !strings.EqualFold(strings.TrimSpace(claims.Tid), home) {
		return tokenClaims{}, authErr(fmt.Errorf("transcripts are own-tenant only: this token is for tenant %s, not %s (%s); a client tenant is refused", claims.Tid, homeTenantEnv, home))
	}
	if claims.Idp != "" && !strings.EqualFold(claims.Idp, claims.Iss) {
		return tokenClaims{}, authErr(fmt.Errorf("transcripts are own-tenant only: this token's user signed in from another tenant (%s), as a GDAP or guest user does; use a member account of your own tenant", claims.Idp))
	}
	if claims.appOnly() && os.Getenv(transcriptsAppOnlyEnv) != "1" {
		return tokenClaims{}, authErr(fmt.Errorf("this is an app-only token; transcripts use a delegated (signed-in user) token by default. To read with an application token in your own tenant, limit the app with a Teams application access policy (New-CsApplicationAccessPolicy, then Grant-CsApplicationAccessPolicy for each user) and set %s=1", transcriptsAppOnlyEnv))
	}
	return claims, nil
}

// transcriptsWireCheck judges the Authorization header each transcripts
// request carries after every header layer has been applied (configuration,
// config file [headers], per-request overrides), so no layer the gate did not
// read can send another tenant's token. A refused request is never sent.
type transcriptsWireCheck struct {
	home string
	next http.RoundTripper
}

func (t transcriptsWireCheck) RoundTrip(req *http.Request) (*http.Response, error) {
	if _, err := transcriptsCredentialClaims(req.Header.Get("Authorization"), t.home); err != nil {
		if req.Body != nil {
			_ = req.Body.Close()
		}
		return nil, err
	}
	return t.next.RoundTrip(req)
}

// meetingTranscriptsPath is the Graph path to one meeting's transcripts: the
// signed-in user's meeting for a delegated token, the named organizer's for an
// app-only one.
func meetingTranscriptsPath(claims tokenClaims, user, meeting string) (string, error) {
	if strings.TrimSpace(meeting) == "" {
		return "", usageErr(errors.New("--meeting is required (the online meeting id)"))
	}
	if claims.appOnly() {
		if strings.TrimSpace(user) == "" {
			return "", usageErr(errors.New("an app-only token needs --user, the meeting organizer's user id"))
		}
		p := replacePathParam("/users/{user}/onlineMeetings/{meeting}/transcripts", "user", user)
		return replacePathParam(p, "meeting", meeting), nil
	}
	if user != "" {
		return "", usageErr(errors.New("--user is for app-only tokens; a delegated token reads the signed-in user's meetings"))
	}
	return replacePathParam("/me/onlineMeetings/{meeting}/transcripts", "meeting", meeting), nil
}
