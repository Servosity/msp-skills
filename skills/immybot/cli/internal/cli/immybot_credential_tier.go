// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
//
// Hand-authored (novel file, not generated). The CLI half of the credential
// tier (internal/cliutil/immybot_credential_tier.go: `access
// get-get-azure-tenant-auth-details-by-azure-tenant-principal-id` returns the
// tenant's custom app registration secret and `access
// create-update-azure-tenant-auth-details` echoes it back).
//
//   - The commands keep working for an operator in a terminal and print the
//     response there. An answer that carries the secret reaches stdout as the
//     credential kit's withheld notice (customAppRegSecret is a declared
//     response field), so the secret itself is read in the ImmyBot console.
//   - They are marked mcp:hidden, so the cobratree mirror and recipe intents
//     never expose them as MCP tools, even if a reprint drops the pp:endpoint
//     classification that keeps endpoint commands out of the mirror today.
//   - Inside a process the MCP server spawned (IMMYBOT_MCP_CHILD=1) they
//     refuse before building a client, naming the terminal command. The
//     client transport refuses the member paths there too, which covers
//     `export access <id>` and any other route onto them.
//   - Their response never reaches the local store: the run carries a context
//     mark (cliutil.WithCredentialTierRun) that writeThroughCache and
//     writeMutationResponseToStore (data_source.go) honor.
//   - The client fetches a member only inside that mark
//     (cliutil.CredentialTierRequestError), so a redirect onto one, `export
//     access <id>` or any other route is refused before it is sent, and the
//     commands refuse --deliver to a file or a webhook: the answer reaches
//     the terminal that asked and nothing else.
//
// Registered through novelCommandHooks, so it survives regeneration of the
// generated command files (handfixes.json:
// credential-tier-never-cached-never-served-to-mcp).

package cli

import (
	"context"
	"errors"
	"strings"

	"github.com/spf13/cobra"

	"immybot-pp-cli/internal/cliutil"
)

const cliOnlyTierAnnotation = "pp:credential-tier"

func init() {
	registerNovelCommand(func(root *cobra.Command, flags *rootFlags) {
		markCredentialTierCommands(root, flags)
	})
}

// credentialTierMember reports the tier member a command calls, by its
// generated pp:endpoint id or its pp:path.
func credentialTierMember(cmd *cobra.Command) (cliutil.CredentialTierEndpoint, bool) {
	if cmd == nil || cmd.Annotations == nil {
		return cliutil.CredentialTierEndpoint{}, false
	}
	return cliutil.CredentialTierFor(cmd.Annotations["pp:endpoint"], strings.TrimSpace(cmd.Annotations["pp:path"]))
}

func markCredentialTierCommands(cmd *cobra.Command, flags *rootFlags) {
	for _, child := range cmd.Commands() {
		markCredentialTierCommands(child, flags)
	}
	ep, ok := credentialTierMember(cmd)
	if !ok || cmd.Annotations[cliOnlyTierAnnotation] == "true" {
		return
	}
	cmd.Annotations["mcp:hidden"] = "true"
	cmd.Annotations[cliOnlyTierAnnotation] = "true"
	note := "CLI only: the response carries " + ep.Credential + ". It is printed here and goes nowhere else: never to --deliver, the response cache or the local store, and this command is never offered over MCP."
	if long := strings.TrimSpace(cmd.Long); long != "" {
		cmd.Long = long + "\n\n" + note
	} else {
		cmd.Long = strings.TrimSpace(cmd.Short) + "\n\n" + note
	}
	enter := func(c *cobra.Command) error {
		if !cliutil.CredentialTierServedHere() {
			return errors.New(cliutil.CredentialTierRefusal(ep, "an MCP-spawned CLI process"))
		}
		if flags != nil && flags.deliverSpec != "" && flags.deliverSpec != "stdout" {
			return usageErr(errors.New(cliutil.CredentialTierRefusal(ep, "--deliver "+flags.deliverSpec)))
		}
		ctx := c.Context()
		if ctx == nil {
			ctx = context.Background()
		}
		c.SetContext(cliutil.WithCredentialTierRun(ctx))
		return nil
	}
	if run := cmd.RunE; run != nil {
		cmd.RunE = func(c *cobra.Command, args []string) error {
			if err := enter(c); err != nil {
				return err
			}
			return run(c, args)
		}
	} else if run := cmd.Run; run != nil {
		cmd.Run = nil
		cmd.RunE = func(c *cobra.Command, args []string) error {
			if err := enter(c); err != nil {
				return err
			}
			run(c, args)
			return nil
		}
	}
}

// credentialTierStoreRefused reports whether ctx belongs to a credential-tier
// command run, whose response never reaches the local store.
func credentialTierStoreRefused(ctx context.Context) bool {
	return cliutil.CredentialTierRun(ctx)
}
