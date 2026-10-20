// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cobratree

import (
	"fmt"
	"sort"
	"strings"

	"eset-protect-pp-cli/internal/cliutil"
	"github.com/spf13/cobra"
)

// credentialTierEndpoints are the ESET Connect reads that governance.md puts
// in the Credential tier: what they return enrolls a device into the tenant
// or hands over a quarantined sample. installers list/get are GETs, so the
// read-only switch alone let an agent fetch them; the POST ones are listed
// too so the tier, not the HTTP method, decides. Refused while
// ESET_PROTECT_MCP_READ_ONLY=1 (handfixes.json: mcp-read-only-refuses-credential-tier).
var credentialTierEndpoints = map[string]string{
	"installers.list": "installer download links install and enroll a device into the tenant",
	"installers.get":  "installer download links install and enroll a device into the tenant",
	"gpo-sccm-files-generate.installers-generate-gpo-sccm-file":                              "GPO and SCCM deployment files enroll devices into the tenant",
	"mobile-devices-batch-get-enrollment-links.v1_mobile_devices_batch_get_enrollment_links": "mobile enrollment links enroll a device into the tenant",
	"quarantined-objects-download.quarantined-objects":                                       "quarantine downloads hand over quarantined samples",
	"quarantined-objects-batch-download.quarantined-objects":                                 "quarantine downloads hand over quarantined samples",
}

// credentialTierResources are the sync resources whose stored rows carry
// Credential-tier material (installers carry downloadUrl).
var credentialTierResources = map[string]string{
	"installers": "installer download links install and enroll a device into the tenant",
}

// CredentialTierReadOnlyRefusal returns the refusal for endpointID while the
// MCP read-only switch is on, or "" when the call may proceed.
func CredentialTierReadOnlyRefusal(endpointID string) string {
	if !cliutil.IsMCPReadOnlyEnv() {
		return ""
	}
	why, ok := credentialTierEndpoints[strings.TrimSpace(endpointID)]
	if !ok {
		return ""
	}
	return fmt.Sprintf("refusing %s while %s=1: it is a Credential-tier read (%s); run it from a terminal or a server without the read-only switch", endpointID, cliutil.MCPReadOnlyEnvVar, why)
}

// CredentialTierResourceNames lists the sync resources whose rows are
// Credential tier, sorted, for the read-only sql and search gate.
func CredentialTierResourceNames() []string {
	out := make([]string, 0, len(credentialTierResources))
	for name := range credentialTierResources {
		out = append(out, name)
	}
	sort.Strings(out)
	return out
}

// credentialTierArgRefusal refuses, in read-only mode, a mirrored command
// that would read a Credential-tier resource: one bound to a listed endpoint,
// or a call whose resource-naming argument (analytics --type, export
// <resource>, sync --resources) names a listed resource.
func credentialTierArgRefusal(cmd *cobra.Command, args map[string]any) string {
	if !cliutil.IsMCPReadOnlyEnv() {
		return ""
	}
	if msg := CredentialTierReadOnlyRefusal(endpointID(cmd)); msg != "" {
		return msg
	}
	path := "command"
	if cmd != nil {
		path = cmd.CommandPath()
	}
	for _, key := range []string{"type", "resource", "resources", "args"} {
		raw, ok := args[key]
		if !ok || raw == nil {
			continue
		}
		for _, value := range strings.FieldsFunc(fmt.Sprint(raw), func(r rune) bool {
			return r == ',' || r == ' ' || r == '\t' || r == '[' || r == ']'
		}) {
			if why, listed := credentialTierResources[strings.ToLower(strings.TrimSpace(value))]; listed {
				return fmt.Sprintf("refusing %s %s=%s while %s=1: %s rows are Credential tier (%s)", path, key, value, cliutil.MCPReadOnlyEnvVar, value, why)
			}
		}
	}
	return ""
}

// esetMirroredCallRefusal is the one call-time gate the mirror applies before
// spawning the CLI: local-file flags in every mode, Credential-tier reads in
// read-only mode.
func esetMirroredCallRefusal(cmd *cobra.Command, args map[string]any) string {
	if msg := localReadRefusal(cmd, args); msg != "" {
		return msg
	}
	return credentialTierArgRefusal(cmd, args)
}
