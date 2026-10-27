// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
//
// Hand-authored (not generated). The credential tier: the one explicit list of
// ThreatLocker reads whose response IS a credential. Every surface consults
// this list and nothing else: the response cache (internal/client), the store
// write-through and the legacy scrub (internal/cli, internal/store), the MCP
// executor, search and get tools, the cobratree mirror and doctor's probe.
//
// A credential-returning read is not an ordinary read. Both members below
// return the material a new agent uses to enroll into the tenant: whoever holds
// it can register a machine under the MSP's customer. An operator who types the
// CLI command in a terminal asked for it and gets it on stdout. It is never
// written to the response cache or the local store, and never returned to an
// MCP client (an agent context, transcript and tool log are not a secret
// store), with or without THREATLOCKER_MCP_READ_ONLY.
//
// Membership is by endpoint id and API path, never by a name heuristic. A test
// (internal/mcp/tl_credential_tier_mcp_test.go) scans the spec manifest and the
// response types for credential-shaped fields as a tripwire, so a reprint that
// adds a new credential read fails a test instead of shipping as an ordinary
// read; the list here is still the only thing that gates.

package cliutil

import (
	"fmt"
	"net/url"
	"path"
	"strings"
	"sync/atomic"
)

// CredentialTierEndpoint is one read whose response carries credential
// material.
type CredentialTierEndpoint struct {
	// ID is the pp:endpoint annotation and the MCP executor endpoint_id.
	ID string
	// Path is the API path, relative to the base URL.
	Path string
	// Command is the CLI command an operator runs in a terminal instead.
	Command string
	// Credential names what the response returns, for refusal messages.
	Credential string
	// StoreResource is the local-store resource the generic write-through
	// files this response under, and StoreFields the response keys and
	// typed-table columns that carry the credential there. They drive the
	// scrub of stores an earlier binary already wrote to.
	StoreResource string
	StoreFields   []CredentialTierStoreField
}

// CredentialTierStoreField pairs a response JSON key with the typed-table
// column the store projects it into.
type CredentialTierStoreField struct {
	JSONKey string
	Column  string
}

var credentialTier = []CredentialTierEndpoint{
	{
		// types.AuthKey{organizationId, authKey}: the organization's agent
		// installation key.
		ID:            "organizations.auth-key",
		Path:          "/Organization/OrganizationGetAuthKeyById",
		Command:       "threatlocker-cli organizations auth-key",
		Credential:    "the organization's agent-install AuthKey",
		StoreResource: "organizations",
		StoreFields:   []CredentialTierStoreField{{JSONKey: "authKey", Column: "auth_key"}},
	},
	{
		// types.InstallInfo{installKey, downloadUrl}: the install key a new
		// computer enrolls with, and the installer URL handed out with it.
		ID:            "computers.install-info",
		Path:          "/Computer/ComputerGetForNewComputer",
		Command:       "threatlocker-cli computers install-info",
		Credential:    "the agent install key and its installer download URL",
		StoreResource: "computers",
		StoreFields: []CredentialTierStoreField{
			{JSONKey: "installKey", Column: "install_key"},
			{JSONKey: "downloadUrl", Column: "download_url"},
		},
	},
}

// CredentialTier returns a copy of the tier.
func CredentialTier() []CredentialTierEndpoint {
	out := make([]CredentialTierEndpoint, len(credentialTier))
	for i, ep := range credentialTier {
		ep.StoreFields = append([]CredentialTierStoreField(nil), ep.StoreFields...)
		out[i] = ep
	}
	return out
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

// CredentialTierByPath returns the member a request path addresses. The match
// is fail-closed: the query string and fragment are ignored, percent-escapes
// are decoded, dot segments and doubled slashes are collapsed, case is folded
// (the Portal API routes case-insensitively), and an absolute URL or a base
// path prefix such as /portalapi still matches on a segment boundary.
func CredentialTierByPath(raw string) (CredentialTierEndpoint, bool) {
	p := normalizeCredentialTierPath(raw)
	if p == "" {
		return CredentialTierEndpoint{}, false
	}
	for _, ep := range credentialTier {
		want := strings.ToLower(ep.Path)
		if p == want || strings.HasSuffix(p, want) {
			return ep, true
		}
	}
	return CredentialTierEndpoint{}, false
}

// IsCredentialTierPath reports whether a request path is a credential read.
func IsCredentialTierPath(raw string) bool {
	_, ok := CredentialTierByPath(raw)
	return ok
}

func normalizeCredentialTierPath(raw string) string {
	s := strings.TrimSpace(raw)
	if i := strings.IndexAny(s, "?#"); i >= 0 {
		s = s[:i]
	}
	if strings.Contains(s, "://") {
		if u, err := url.Parse(s); err == nil {
			s = u.Path
		}
	}
	for i := 0; i < 3 && strings.Contains(s, "%"); i++ {
		decoded, err := url.PathUnescape(s)
		if err != nil {
			break
		}
		s = decoded
	}
	s = strings.ReplaceAll(s, "\\", "/")
	if s == "" {
		return ""
	}
	s = path.Clean("/" + s)
	return strings.ToLower(strings.TrimRight(s, "/"))
}

// CredentialTierRefusal is the message every refusing surface shows.
func CredentialTierRefusal(ep CredentialTierEndpoint, surface string) string {
	return fmt.Sprintf("refusing %s over %s: it returns %s, a credential that enrolls agents into the tenant, so it is never returned to an MCP client, cached or stored. Run `%s` in a terminal to read it.",
		ep.ID, surface, ep.Credential, ep.Command)
}

var mcpServerProcess atomic.Bool

// MarkMCPServerProcess records that this process is the MCP server. It is
// called from package mcp's init, so it holds in the threatlocker-mcp binary
// and never in threatlocker-cli, which does not link that package.
func MarkMCPServerProcess() { mcpServerProcess.Store(true) }

// CredentialTierServedHere reports whether this process may fetch a
// credential-tier read at all: not inside the MCP server, and not inside a
// CLI process the MCP server spawned (THREATLOCKER_MCP_CHILD=1).
func CredentialTierServedHere() bool {
	return !mcpServerProcess.Load() && !IsMCPChildEnv()
}
