// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
//
// Hand-authored (not generated). The credential tier: the one explicit list of
// ImmyBot endpoints whose response IS a credential. Every surface consults
// this list and nothing else: the MCP executor, search and get tools
// (internal/mcp/code_orch.go), the client transport and response cache
// (internal/client/client.go), the local-store writers and the command marks
// that keep the cobratree mirror and recipe intents off it
// (internal/cli/immybot_credential_tier.go).
//
// Both members carry AzureTenantAuthDetails.customAppRegSecret (spec.json
// components.schemas.AzureTenantAuthDetails), the client secret of the
// tenant's custom Azure app registration: with the app id returned beside it,
// whoever holds it can sign in to the customer's Entra tenant as that app. The
// GET returns it; the update echoes the stored details back under authDetails
// (UpdateAzureTenantAuthDetailsCmdResult). It is the only credential value in
// any response schema of the spec.
//
// An operator who types the CLI command in a terminal asked for it and gets it
// on stdout. It is never written to the response cache or the local store,
// and never returned to an MCP client (an agent context, transcript and tool
// log are not a secret store), with or without IMMYBOT_MCP_READ_ONLY.
//
// Membership is by endpoint id and API path, never by a field-name heuristic.
// A reprint that adds a credential read has to add it here.

package cliutil

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"path"
	"strings"
	"sync/atomic"
)

// CredentialTierEndpoint is one endpoint whose response carries credential
// material.
type CredentialTierEndpoint struct {
	// ID is the pp:endpoint annotation and the MCP executor endpoint_id.
	ID string
	// Method and Path are the spec operation; {name} is one path segment.
	Method string
	Path   string
	// Command is the CLI command an operator runs in a terminal instead.
	Command string
	// Credential names what the response returns, for refusal messages.
	Credential string
}

var credentialTier = []CredentialTierEndpoint{
	{
		// AzureTenantAuthDetails{azureTenantPrincipalId, selectedPermissionLevel,
		// customAppRegAppId, customAppRegSecret}.
		ID:         "access.get-get-azure-tenant-auth-details-by-azure-tenant-principal-id",
		Method:     "GET",
		Path:       "/api/v1/auth/get-azure-tenant-auth-details/{azureTenantPrincipalId}",
		Command:    "immybot-cli access get-get-azure-tenant-auth-details-by-azure-tenant-principal-id <azureTenantPrincipalId>",
		Credential: "the client secret of the tenant's custom Azure app registration (customAppRegSecret)",
	},
	{
		// UpdateAzureTenantAuthDetailsCmdResult{authDetails, defaultAppRegistrationUpdated}:
		// the stored details echoed back, secret included.
		ID:         "access.create-update-azure-tenant-auth-details",
		Method:     "POST",
		Path:       "/api/v1/auth/update-azure-tenant-auth-details",
		Command:    "immybot-cli access create-update-azure-tenant-auth-details",
		Credential: "the client secret of the tenant's custom Azure app registration (customAppRegSecret), echoed back under authDetails",
	},
}

// CredentialTier returns a copy of the tier.
func CredentialTier() []CredentialTierEndpoint {
	return append([]CredentialTierEndpoint(nil), credentialTier...)
}

// CredentialTierByID returns the member with the given endpoint id.
func CredentialTierByID(id string) (CredentialTierEndpoint, bool) {
	id = strings.TrimSpace(id)
	for _, ep := range credentialTier {
		if strings.EqualFold(ep.ID, id) {
			return ep, true
		}
	}
	return CredentialTierEndpoint{}, false
}

// CredentialTierFor returns the member an endpoint id or a request path names.
func CredentialTierFor(id, rawPath string) (CredentialTierEndpoint, bool) {
	if ep, ok := CredentialTierByID(id); ok {
		return ep, true
	}
	return CredentialTierByPath(rawPath)
}

// CredentialTierByPath returns the member a request path addresses, for any
// method. The match fails closed: the query string and fragment are ignored,
// case is folded (ASP.NET Core routes case-insensitively), dot segments and
// doubled slashes are collapsed, and the path is tried both as escaped
// segments and fully percent-decoded, so a %2F inside a segment cannot hide
// it. An absolute URL, a base path, or an /api/vN prefix other than the
// spec's still matches, because only the member's distinctive tail segments
// must appear, contiguously.
func CredentialTierByPath(raw string) (CredentialTierEndpoint, bool) {
	for _, segs := range credentialTierPathForms(raw) {
		for _, ep := range credentialTier {
			if credentialTierSegmentsContain(segs, credentialTierPattern(ep.Path)) {
				return ep, true
			}
		}
	}
	return CredentialTierEndpoint{}, false
}

// IsCredentialTierPath reports whether a request path is a member.
func IsCredentialTierPath(raw string) bool {
	_, ok := CredentialTierByPath(raw)
	return ok
}

// credentialTierPattern is the member path without its /api/vN prefix, as
// lower-case segments; "" stands for a {placeholder}.
func credentialTierPattern(template string) []string {
	segs := strings.Split(strings.Trim(strings.ToLower(template), "/"), "/")
	if len(segs) > 2 && segs[0] == "api" && strings.HasPrefix(segs[1], "v") {
		segs = segs[2:]
	}
	for i, s := range segs {
		if strings.HasPrefix(s, "{") && strings.HasSuffix(s, "}") {
			segs[i] = ""
		}
	}
	return segs
}

func credentialTierSegmentsContain(segs, pattern []string) bool {
	for i := 0; i+len(pattern) <= len(segs); i++ {
		match := true
		for j, want := range pattern {
			got := segs[i+j]
			if got == "" || (want != "" && got != want) {
				match = false
				break
			}
		}
		if match {
			return true
		}
	}
	return false
}

// credentialTierPathForms returns raw as cleaned lower-case segments twice:
// split before decoding (each segment then decoded) and fully decoded first.
func credentialTierPathForms(raw string) [][]string {
	s := strings.TrimSpace(raw)
	if strings.Contains(s, "://") {
		if u, err := url.Parse(s); err == nil {
			s = u.EscapedPath()
		}
	}
	if i := strings.IndexAny(s, "?#"); i >= 0 {
		s = s[:i]
	}
	s = strings.ToLower(strings.ReplaceAll(s, "\\", "/"))
	if s == "" {
		return nil
	}
	var bySegment []string
	for _, seg := range strings.Split(path.Clean("/"+s), "/") {
		if decoded, err := url.PathUnescape(seg); err == nil {
			seg = decoded
		}
		if seg != "" {
			bySegment = append(bySegment, seg)
		}
	}
	decoded := s
	for i := 0; i < 3 && strings.Contains(decoded, "%"); i++ {
		next, err := url.PathUnescape(decoded)
		if err != nil {
			break
		}
		decoded = next
	}
	decoded = strings.ReplaceAll(decoded, "\\", "/")
	var whole []string
	for _, seg := range strings.Split(path.Clean("/"+decoded), "/") {
		if seg != "" {
			whole = append(whole, seg)
		}
	}
	return [][]string{bySegment, whole}
}

// CredentialTierRefusal is the message every refusing surface shows.
func CredentialTierRefusal(ep CredentialTierEndpoint, surface string) string {
	return fmt.Sprintf("refusing %s over %s: it returns %s, so it is CLI only: never cached or stored, never offered over MCP. Run `%s` in a terminal instead.",
		ep.ID, surface, ep.Credential, ep.Command)
}

var mcpServerProcess atomic.Bool

// MarkMCPServerProcess records that this process is the MCP server. Package
// mcp calls it from init, so it holds in immybot-mcp and never in
// immybot-cli, which does not link that package.
func MarkMCPServerProcess() { mcpServerProcess.Store(true) }

// CredentialTierServedHere reports whether this process may fetch a member at
// all: not inside the MCP server, and not inside a CLI process it spawned
// (IMMYBOT_MCP_CHILD=1).
func CredentialTierServedHere() bool {
	return !mcpServerProcess.Load() && !IsMCPChildEnv()
}

// CredentialTierTransportError refuses a request for a member, matched on any
// of the given spellings of its target, when this process may not serve it.
func CredentialTierTransportError(targets ...string) error {
	if CredentialTierServedHere() {
		return nil
	}
	for _, target := range targets {
		if ep, ok := CredentialTierByPath(target); ok {
			return errors.New(CredentialTierRefusal(ep, "MCP"))
		}
	}
	return nil
}

type credentialTierRunKey struct{}

// WithCredentialTierRun marks ctx as a run of a credential-tier command: the
// only context a member may be fetched in.
func WithCredentialTierRun(ctx context.Context) context.Context {
	return context.WithValue(ctx, credentialTierRunKey{}, true)
}

// CredentialTierRun reports whether ctx belongs to a credential-tier command
// run, whose response reaches the terminal and nothing else.
func CredentialTierRun(ctx context.Context) bool {
	if ctx == nil {
		return false
	}
	run, _ := ctx.Value(credentialTierRunKey{}).(bool)
	return run
}

// CredentialTierRequestError refuses a request for a member, matched on any
// of the given spellings of its target, unless this process may serve it and
// ctx is a run of a tier command. A redirect onto a member, or any other
// command reaching one (export, a recipe), is refused before it is sent.
func CredentialTierRequestError(ctx context.Context, targets ...string) error {
	if err := CredentialTierTransportError(targets...); err != nil {
		return err
	}
	if CredentialTierRun(ctx) {
		return nil
	}
	for _, target := range targets {
		if ep, ok := CredentialTierByPath(target); ok {
			return errors.New(CredentialTierRefusal(ep, "a command other than its own, or a redirect"))
		}
	}
	return nil
}
