// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-written: fleet add/list/remove manage the local PBX registry.
// pp:data-source local

package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"threecx-pp-cli/internal/cliutil"
	"threecx-pp-cli/internal/fleet"
)

type fleetListRow struct {
	Name      string   `json:"name"`
	FQDN      string   `json:"fqdn"`
	ClientID  string   `json:"client_id"`
	EnvPrefix string   `json:"env_prefix"`
	SecretVar string   `json:"secret_env"`
	SecretSet bool     `json:"secret_set"`
	Tags      []string `json:"tags"`
}

func listRow(p fleet.PBX) fleetListRow {
	tags := p.Tags
	if tags == nil {
		tags = []string{}
	}
	return fleetListRow{Name: p.Name, FQDN: p.FQDN, ClientID: p.ClientID, EnvPrefix: p.EnvPrefix, SecretVar: p.SecretVarName(), SecretSet: os.Getenv(p.EnvPrefix+"_CLIENT_SECRET") != "", Tags: tags}
}

func newFleetAddCmd(flags *rootFlags) *cobra.Command {
	var fqdn, clientID, envPrefix string
	var tags []string
	cmd := &cobra.Command{
		Use:   "add <name>",
		Short: "Register (or update) one client's 3CX phone system and the env var that holds its secret",
		Long: `Register (or update) one 3CX phone system in the local fleet file.

The client ID is the DN number of the API client created in the PBX's Admin
Console > Integrations > API. The client secret (the API key shown once) is
never stored: it is read from <PREFIX>_CLIENT_SECRET, where --env-prefix sets
<PREFIX> (default THREECX_<NAME>, so THREECX_ACME_CLIENT_SECRET for acme). Use host:port for on-premise systems that do
not listen on 443.`,
		Example: strings.Trim(`
  threecx-cli fleet add acme --fqdn acme.3cx.us --client-id 900 --env-prefix THREECX_ACME
  threecx-cli fleet add globex --fqdn pbx.globex.example:5001 --client-id 901 --tag onprem,gold`, "\n"),
		// Hidden from MCP: re-pointing a PBX at another host would send its
		// API key there, so registry changes stay an operator action.
		Annotations: map[string]string{
			"pp:data-source": "local",
			"mcp:hidden":     "true",
			"pp:happy-args":  "name=acme;--fqdn=acme.3cx.us;--client-id=900;--env-prefix=THREECX_ACME",
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 && cmd.Flags().NFlag() == 0 {
				return cmd.Help()
			}
			// Harness dry-runs probe with synthetic args only; a user's
			// --dry-run still validates like a real add.
			if dryRunOK(flags) && (len(args) == 0 || requiredInputWaived()) {
				return writeDryRun(cmd.OutOrStdout(), flags, "fleet add")
			}
			if len(args) != 1 {
				_ = cmd.Usage()
				return usageErr(fmt.Errorf("exactly one PBX name is required"))
			}
			p := fleet.PBX{Name: strings.TrimSpace(args[0]), FQDN: strings.TrimSpace(fqdn), ClientID: strings.TrimSpace(clientID),
				EnvPrefix: strings.TrimSpace(envPrefix), Tags: tags}
			if p.EnvPrefix == "" {
				p.EnvPrefix = fleet.DefaultEnvPrefix(p.Name)
			}
			if err := p.Validate(); err != nil {
				return usageErr(err)
			}
			path, err := fleet.Path()
			if err != nil {
				return configErr(err)
			}
			result := map[string]any{"pbx": listRow(p), "path": path}
			if dryRunOK(flags) {
				result["dry_run"] = true
				return printJSONFiltered(cmd.OutOrStdout(), result, flags)
			}
			if cliutil.IsAnyHarness() {
				return writeHarnessRefusal(cmd.OutOrStdout(), flags, "fleet add")
			}
			err = fleet.Update(func(reg *fleet.Registry) error {
				for _, other := range reg.PBXes {
					if other.Name != p.Name && other.EnvPrefix == p.EnvPrefix {
						return usageErr(fmt.Errorf("env prefix %s is already used by PBX %q; pass a distinct --env-prefix", p.EnvPrefix, other.Name))
					}
				}
				result["replaced"] = reg.Upsert(p)
				return nil
			}, func(base string) error {
				// A re-pointed PBX gets a new identity home; drop the old one
				// and the token cached in it.
				return fleet.PruneHomes(base, p)
			})
			if err != nil {
				var ce *cliError
				if errors.As(err, &ce) {
					return err
				}
				return configErr(err)
			}
			result["saved"] = true
			if !wantsHumanTable(cmd.OutOrStdout(), flags) {
				return printJSONFiltered(cmd.OutOrStdout(), result, flags)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "saved %s (%s) to %s\n", p.Name, p.FQDN, path)
			if os.Getenv(p.EnvPrefix+"_CLIENT_SECRET") == "" {
				fmt.Fprintf(cmd.OutOrStdout(), "next: export %s=<api key from Admin Console > Integrations > API>\n", p.SecretVarName())
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&fqdn, "fqdn", "", "PBX host name, optionally host:port (e.g. acme.3cx.us or pbx.example.com:5001)")
	cmd.Flags().StringVar(&clientID, "client-id", "", "API client ID: the DN number shown in Integrations > API")
	cmd.Flags().StringVar(&envPrefix, "env-prefix", "", "Prefix of the variable holding this PBX's API key, read as <PREFIX>_CLIENT_SECRET (default THREECX_<NAME>)")
	cmd.Flags().StringSliceVar(&tags, "tag", nil, "Tags for grouping, e.g. onprem,gold (comma-separated)")
	return cmd
}

func newFleetListCmd(flags *rootFlags) *cobra.Command {
	cmd := &cobra.Command{
		Use:         "list",
		Short:       "List registered phone systems and whether each secret env var is set",
		Example:     "  threecx-cli fleet list --json",
		Annotations: map[string]string{"pp:data-source": "local", "mcp:read-only": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return writeDryRun(cmd.OutOrStdout(), flags, "fleet list")
			}
			reg, err := fleet.Load()
			if err != nil {
				return configErr(err)
			}
			rows := make([]fleetListRow, 0, len(reg.PBXes))
			for _, p := range reg.PBXes {
				// Under --pbx the other PBXes' secrets were scrubbed from this
				// process, so only the pinned entry can report secret_set.
				if pinnedPBX == "" || p.Name == pinnedPBX {
					rows = append(rows, listRow(p))
				}
			}
			if !wantsHumanTable(cmd.OutOrStdout(), flags) {
				return printJSONFiltered(cmd.OutOrStdout(), rows, flags)
			}
			if len(rows) == 0 {
				hintEmptyFleet(cmd)
				return nil
			}
			data, _ := json.Marshal(rows)
			var items []map[string]any
			_ = json.Unmarshal(data, &items)
			return printAutoTable(cmd.OutOrStdout(), items)
		},
	}
	return cmd
}

func newFleetRemoveCmd(flags *rootFlags) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "remove <name>",
		Short: "Remove a phone system from the fleet and delete its cached token and local data",
		Example: strings.Trim(`
  threecx-cli fleet remove acme`, "\n"),
		Annotations: map[string]string{"pp:data-source": "local", "mcp:hidden": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 && cmd.Flags().NFlag() == 0 {
				return cmd.Help()
			}
			if dryRunOK(flags) {
				return writeDryRun(cmd.OutOrStdout(), flags, "fleet remove")
			}
			if cliutil.IsAnyHarness() {
				return writeHarnessRefusal(cmd.OutOrStdout(), flags, "fleet remove")
			}
			if len(args) != 1 {
				_ = cmd.Usage()
				return usageErr(fmt.Errorf("exactly one PBX name is required"))
			}
			name := strings.TrimSpace(args[0])
			found, err := fleet.RemovePBX(name)
			if err != nil {
				return configErr(err)
			}
			if !found {
				reg, _ := fleet.Load()
				names := []string{}
				if reg != nil {
					names = reg.Names()
				}
				return notFoundErr(fmt.Errorf("no PBX named %q (registered: %s)", name, strings.Join(names, ", ")))
			}
			return printJSONFiltered(cmd.OutOrStdout(), map[string]any{"removed": name, "purged_local_data": true}, flags)
		},
	}
	return cmd
}
