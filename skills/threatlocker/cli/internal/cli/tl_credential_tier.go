// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
//
// Hand-authored (novel file, not generated). The CLI half of the credential
// tier (internal/cliutil/tl_credential_tier.go: `organizations auth-key` and
// `computers install-info` return agent-enrollment credentials).
//
//   - The commands keep working for an operator in a terminal.
//   - They are marked mcp:hidden, so the cobratree mirror and recipe intents
//     never expose them as MCP tools, even if a reprint drops the pp:endpoint
//     classification that keeps endpoint commands out of the mirror today.
//   - Inside a process the MCP server spawned (THREATLOCKER_MCP_CHILD=1) they
//     refuse before building a client, with a message naming the terminal
//     command. The client transport refuses the same paths there too.
//   - Their live response is never written through to the local store:
//     writeThroughCacheUnlessCredentialTier replaces both generated
//     writeThroughCache call sites in data_source.go.
//
// Registered through novelCommandHooks, so it survives regeneration of the
// generated command files (handfixes.json:
// credential-tier-never-cached-never-served-to-mcp).

package cli

import (
	"context"
	"encoding/json"
	"errors"
	"strings"

	"github.com/spf13/cobra"

	"threatlocker-pp-cli/internal/cliutil"
)

const enrollmentTierAnnotation = "pp:credential-tier"

func init() {
	registerNovelCommand(func(root *cobra.Command, _ *rootFlags) {
		markCredentialTierCommands(root)
	})
}

// credentialTierMember reports the tier member a command reads, by its
// generated pp:endpoint id or its pp:path.
func credentialTierMember(cmd *cobra.Command) (cliutil.CredentialTierEndpoint, bool) {
	if cmd == nil || cmd.Annotations == nil {
		return cliutil.CredentialTierEndpoint{}, false
	}
	if ep, ok := cliutil.CredentialTierByID(cmd.Annotations["pp:endpoint"]); ok {
		return ep, true
	}
	if p := strings.TrimSpace(cmd.Annotations["pp:path"]); p != "" {
		return cliutil.CredentialTierByPath(p)
	}
	return cliutil.CredentialTierEndpoint{}, false
}

func markCredentialTierCommands(cmd *cobra.Command) {
	for _, child := range cmd.Commands() {
		markCredentialTierCommands(child)
	}
	ep, ok := credentialTierMember(cmd)
	if !ok || cmd.Annotations[enrollmentTierAnnotation] == "true" {
		return
	}
	cmd.Annotations["mcp:hidden"] = "true"
	cmd.Annotations[enrollmentTierAnnotation] = "true"
	refuse := func() error {
		if cliutil.CredentialTierServedHere() {
			return nil
		}
		return errors.New(cliutil.CredentialTierRefusal(ep, "an MCP-spawned CLI process"))
	}
	if run := cmd.RunE; run != nil {
		cmd.RunE = func(c *cobra.Command, args []string) error {
			if err := refuse(); err != nil {
				return err
			}
			return run(c, args)
		}
	} else if run := cmd.Run; run != nil {
		cmd.Run = nil
		cmd.RunE = func(c *cobra.Command, args []string) error {
			if err := refuse(); err != nil {
				return err
			}
			run(c, args)
			return nil
		}
	}
}

// writeThroughCacheUnlessCredentialTier is writeThroughCache for every read
// except a credential-tier one, whose response never reaches the store.
func writeThroughCacheUnlessCredentialTier(ctx context.Context, resourceType, path string, data json.RawMessage) {
	if cliutil.IsCredentialTierPath(path) {
		return
	}
	writeThroughCache(ctx, resourceType, data)
}
