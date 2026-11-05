// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-written: core add registers (or updates) one PRTG core.
// pp:data-source local

package cli

import (
	"errors"
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"prtg-pp-cli/internal/cliutil"
	"prtg-pp-cli/internal/fleet"
)

func newCoreAddCmd(flags *rootFlags) *cobra.Command {
	var server string
	var tags []string
	var insecure bool
	cmd := &cobra.Command{
		Use:   "add <name>",
		Short: "Register (or update) one PRTG core; its API key goes in PRTG_FLEET_KEYS",
		Long: `Register (or update) one PRTG core server in the local fleet file.

--server is the host the PRTG API v2 listens on, with the port when it is not
443 (the PRTG application server defaults to 1616). The API key (PRTG: Account
Settings > My Account > API Keys) is never stored: add <name>=<key> to the
PRTG_FLEET_KEYS environment variable, one entry per core, separated by commas
or newlines (e.g. acme=<key>,globex=<key>).`,
		Example: strings.Trim(`
  prtg-cli core add acme --server prtg.acme.example:1616
  prtg-cli core add globex --server 10.20.0.5:1616 --tag onprem,gold`, "\n"),
		// Hidden from MCP: re-pointing a core at another host would send its
		// API key there, so registry changes stay an operator action.
		Annotations: map[string]string{
			"pp:data-source": "local",
			"mcp:hidden":     "true",
			"pp:happy-args":  "name=acme;--server=prtg.acme.example:1616",
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 && cmd.Flags().NFlag() == 0 {
				return cmd.Help()
			}
			// Harness dry-runs probe with synthetic args only; a user's
			// --dry-run still validates like a real add.
			if dryRunOK(flags) && (len(args) == 0 || requiredInputWaived()) {
				return writeDryRun(cmd.OutOrStdout(), flags, "core add")
			}
			if len(args) != 1 {
				_ = cmd.Usage()
				return usageErr(fmt.Errorf("exactly one core name is required"))
			}
			req := fleet.Core{Name: strings.TrimSpace(args[0]), Server: strings.TrimSpace(server),
				Tags: tags, InsecureTLS: insecure}
			// merge applies the request to the registered entry (if any):
			// updating an existing core keeps the fields the caller left out.
			merge := func(reg *fleet.Registry) (fleet.Core, error) {
				c := req
				if old, ok := reg.Get(c.Name); ok {
					if c.Server == "" {
						c.Server = old.Server
					}
					if !cmd.Flags().Changed("tag") {
						c.Tags = old.Tags
					}
					if !cmd.Flags().Changed("insecure-tls") {
						c.InsecureTLS = old.InsecureTLS
					}
				}
				if err := c.Validate(); err != nil {
					return c, usageErr(err)
				}
				return c, nil
			}
			// Preview from the current registry; the real write re-merges
			// under the registry lock so a concurrent update is never undone.
			reg, err := fleet.Load()
			if err != nil {
				return configErr(err)
			}
			c, err := merge(reg)
			if err != nil {
				return err
			}
			path, err := fleet.Path()
			if err != nil {
				return configErr(err)
			}
			keys, keysErr := fleet.Keys()
			result := map[string]any{"core": coreRow(c, keys), "path": path}
			if dryRunOK(flags) {
				result["dry_run"] = true
				return printJSONFiltered(cmd.OutOrStdout(), result, flags)
			}
			if cliutil.IsAnyHarness() {
				return writeHarnessRefusal(cmd.OutOrStdout(), flags, "core add")
			}
			err = fleet.Update(func(reg *fleet.Registry) error {
				merged, err := merge(reg)
				if err != nil {
					return err
				}
				c = merged
				result["core"] = coreRow(c, keys)
				result["replaced"] = reg.Upsert(c)
				return nil
			}, func(base string) error {
				// A re-pointed core gets a new identity home; drop the old one
				// and the mirror cached in it.
				return fleet.PruneHomes(base, c)
			})
			if err != nil {
				var ce *cliError
				if errors.As(err, &ce) {
					return err
				}
				return configErr(err)
			}
			result["saved"] = true
			if keysErr != nil {
				fmt.Fprintf(cmd.ErrOrStderr(), "warning: %v\n", keysErr)
			}
			if !wantsHumanTable(cmd.OutOrStdout(), flags) {
				return printJSONFiltered(cmd.OutOrStdout(), result, flags)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "saved %s (%s) to %s\n", c.Name, c.Server, path)
			if keys[c.Name] == "" {
				fmt.Fprintf(cmd.OutOrStdout(), "next: add %s=<API key from Account Settings > My Account > API Keys> to PRTG_FLEET_KEYS\n", c.Name)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&server, "server", "", "PRTG core host, optionally host:port (e.g. prtg.acme.example:1616)")
	cmd.Flags().StringSliceVar(&tags, "tag", nil, "Tags for grouping, e.g. onprem,gold (comma-separated)")
	cmd.Flags().BoolVar(&insecure, "insecure-tls", false, "Skip TLS certificate verification for this core only (self-signed certificate)")
	return cmd
}
