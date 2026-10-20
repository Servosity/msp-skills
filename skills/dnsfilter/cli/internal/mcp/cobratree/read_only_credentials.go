// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
//
// Hand-written (NOT generated): pinned by skills/dnsfilter/handfixes.json
// under the "mcp-read-only-withholds-credentials" entry.

package cobratree

import (
	"strings"

	"github.com/spf13/cobra"
)

// credentialPathFragments mark DNSFilter API paths whose response IS a
// credential, so no read-only MCP surface may call them: /v1/api_keys and
// /v1/api_keys/{id} return ApiKey.token, /v1/networks/{id}/secret_key manages
// the site deployment secret, and /v1/user_agents/uninstall_pin returns the
// roaming client uninstall PIN. A path that merely carries a secret among
// other fields (the networks lists, enterprise connections) stays callable;
// the MCP result funnel withholds the secret-named field instead.
var credentialPathFragments = []string{
	"/api_keys",
	"/secret_key",
	"/uninstall_pin",
}

// writesBehindGET lists GET paths that change tenant state, so read-only mode
// refuses them even though the method reads as safe.
var writesBehindGET = map[string]string{
	// "Creates a new PSA integration, if one does not exist and returns the
	// redirect link."
	"/v1/psa_integrations/redirect_link": "creates a PSA integration",
}

// ReadOnlyCredentialRefusal returns a non-empty reason when method+path must
// not run while the MCP read-only switch is on: every non-GET, every
// credential-returning path, and every GET that writes. pathTemplate is the
// registered template (with {placeholders}), never a resolved path, so a
// caller-supplied id cannot steer the check.
func ReadOnlyCredentialRefusal(method, pathTemplate string) string {
	lower := strings.ToLower(pathTemplate)
	for _, frag := range credentialPathFragments {
		if strings.Contains(lower, frag) {
			return "it returns or manages a credential"
		}
	}
	if why, ok := writesBehindGET[lower]; ok {
		return why
	}
	if !strings.EqualFold(method, "GET") {
		return "it is a " + strings.ToUpper(method) + " request"
	}
	return ""
}

// commandCallsCredentialPath reports whether a Cobra command declares (via the
// generated pp:method/pp:path annotations) an API call ReadOnlyCredentialRefusal
// refuses. Endpoint commands are never mirrored today; this keeps a future
// mirror of one (or a hand-written command carrying the annotations) off the
// read-only surface too.
func commandCallsCredentialPath(cmd *cobra.Command) bool {
	if cmd == nil || cmd.Annotations == nil {
		return false
	}
	path := cmd.Annotations["pp:path"]
	if path == "" {
		return false
	}
	method := cmd.Annotations["pp:method"]
	if method == "" {
		method = "GET"
	}
	return ReadOnlyCredentialRefusal(method, path) != ""
}

// credentialResourceArg returns the first argument value of a mirrored call
// that names a credential resource by the CLI's resource naming (api-keys is
// /api_keys, secret-key is /secret_key, uninstall-pin is /uninstall_pin), so
// `tail api-keys` cannot poll the API key list under the read-only switch.
// Every string value is checked (flags, structured positionals, and each
// token of the raw args field), comma lists included.
func credentialResourceArg(args map[string]any) string {
	var check func(v any) string
	check = func(v any) string {
		switch tv := v.(type) {
		case string:
			for _, tok := range SplitShellArgs(strings.ReplaceAll(tv, ",", " ")) {
				name := "/" + strings.ReplaceAll(strings.ToLower(strings.TrimSpace(tok)), "-", "_")
				for _, frag := range credentialPathFragments {
					if name == frag {
						return tok
					}
				}
			}
		case []any:
			for _, item := range tv {
				if hit := check(item); hit != "" {
					return hit
				}
			}
		}
		return ""
	}
	for _, v := range args {
		if hit := check(v); hit != "" {
			return hit
		}
	}
	return ""
}
