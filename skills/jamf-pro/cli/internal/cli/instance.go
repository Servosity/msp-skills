// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-written: instance add/list/remove/test manage the local registry of client Jamf Pro instances.
// pp:data-source local

package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"jamf-pro-pp-cli/internal/cliutil"
	"jamf-pro-pp-cli/internal/fleet"
)

type instanceListRow struct {
	Name      string   `json:"name"`
	Server    string   `json:"server"`
	ClientID  string   `json:"client_id"`
	SecretSet bool     `json:"secret_set"`
	Tags      []string `json:"tags"`
}

func instanceRow(p fleet.Instance) instanceListRow {
	tags := p.Tags
	if tags == nil {
		tags = []string{}
	}
	return instanceListRow{Name: p.Name, Server: p.Server, ClientID: p.ClientID, SecretSet: fleet.HasSecret(p.Name), Tags: tags}
}

func newNovelInstanceCmd(flags *rootFlags) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "instance",
		Short: "Register each client's Jamf Pro instance once, then target it with --instance or report on all of them with fleet",
		Long: `Register each client's Jamf Pro instance once (instance add): its server, the
client ID of a Jamf Pro API client, and tags. Each client secret goes in the
one JAMF_PRO_FLEET_SECRETS variable as name=secret entries separated by commas;
secrets are never written to disk.

Then run any command against one instance with the global --instance flag, or
against all of them at once with the fleet commands.`,
		Example:     "  jamf-pro-cli instance list --json",
		Annotations: map[string]string{"pp:data-source": "local", "mcp:read-only": "true"},
		RunE:        parentNoSubcommandRunE(flags),
	}
	addNovelCommandIfAbsent(cmd, newInstanceAddCmd(flags))
	addNovelCommandIfAbsent(cmd, newInstanceListCmd(flags))
	addNovelCommandIfAbsent(cmd, newInstanceRemoveCmd(flags))
	addNovelCommandIfAbsent(cmd, newInstanceTestCmd(flags))
	return cmd
}

func newInstanceAddCmd(flags *rootFlags) *cobra.Command {
	var server, clientID string
	var tags []string
	cmd := &cobra.Command{
		Use:   "add <name>",
		Short: "Register (or update) one client's Jamf Pro instance; its client secret goes in JAMF_PRO_FLEET_SECRETS",
		Long: `Register (or update) one Jamf Pro instance in the local fleet file.

Create the API client in Jamf Pro under Settings > System > API roles and
clients, with an API role holding the read privileges you need. The client
secret is never stored: put it in the JAMF_PRO_FLEET_SECRETS environment
variable as <name>=<secret>, one entry per instance separated by commas. Use
host:port for self-hosted servers that do not listen on 443.`,
		Example: strings.Trim(`
  jamf-pro-cli instance add acme --server acme.jamfcloud.com --client-id 6b1a0d2e-1111-4c3e-9f00-000000000001
  jamf-pro-cli instance add globex --server jamf.globex.example:8443 --client-id 6b1a0d2e-2222-4c3e-9f00-000000000002 --tag onprem,gold`, "\n"),
		// Hidden from MCP: re-pointing an instance at another host would send
		// its client secret there, so registry changes stay an operator action.
		Annotations: map[string]string{
			"pp:data-source": "local",
			"mcp:hidden":     "true",
			"pp:happy-args":  "name=acme;--server=acme.jamfcloud.com;--client-id=6b1a0d2e-1111-4c3e-9f00-000000000001",
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 && cmd.Flags().NFlag() == 0 {
				return cmd.Help()
			}
			// Harness dry-runs probe with synthetic args only; a user's
			// --dry-run still validates like a real add.
			if dryRunOK(flags) && (len(args) == 0 || requiredInputWaived()) {
				return writeDryRun(cmd.OutOrStdout(), flags, "instance add")
			}
			if len(args) != 1 {
				_ = cmd.Usage()
				return usageErr(fmt.Errorf("exactly one instance name is required"))
			}
			p := fleet.Instance{Name: strings.TrimSpace(args[0]), Server: strings.TrimSpace(server), ClientID: strings.TrimSpace(clientID),
				Tags: tags}
			if err := p.Validate(); err != nil {
				return usageErr(err)
			}
			path, err := fleet.Path()
			if err != nil {
				return configErr(err)
			}
			result := map[string]any{"instance": instanceRow(p), "path": path}
			if dryRunOK(flags) {
				result["dry_run"] = true
				return printJSONFiltered(cmd.OutOrStdout(), result, flags)
			}
			if cliutil.IsAnyHarness() {
				return writeHarnessRefusal(cmd.OutOrStdout(), flags, "instance add")
			}
			// Re-registering is refused while a command acts as the instance.
			err = fleet.WithInstanceExclusive(p.Name, func(string) error {
				return fleet.Update(func(reg *fleet.Registry) error {
					result["replaced"] = reg.Upsert(p)
					return nil
				}, func(base string) error {
					// A re-pointed instance gets a new identity home; drop the old
					// one and the token cached in it.
					return fleet.PruneHomes(base, p)
				})
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
			fmt.Fprintf(cmd.OutOrStdout(), "saved %s (%s) to %s\n", p.Name, p.Server, path)
			if !fleet.HasSecret(p.Name) {
				fmt.Fprintf(cmd.OutOrStdout(), "next: add %s=<client secret of the Jamf Pro API client> to %s (entries separated by commas)\n", p.Name, fleet.EntriesEnv)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&server, "server", "", "Jamf Pro host, optionally host:port (e.g. acme.jamfcloud.com or jamf.example.com:8443)")
	cmd.Flags().StringVar(&clientID, "client-id", "", "Client ID of the Jamf Pro API client (Settings > System > API roles and clients)")
	cmd.Flags().StringSliceVar(&tags, "tag", nil, "Tags for grouping, e.g. onprem,gold (comma-separated)")
	return cmd
}

func newInstanceListCmd(flags *rootFlags) *cobra.Command {
	cmd := &cobra.Command{
		Use:         "list",
		Short:       "List registered Jamf Pro instances and whether each secret env var is set",
		Example:     "  jamf-pro-cli instance list --json",
		Annotations: map[string]string{"pp:data-source": "local", "mcp:read-only": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return writeDryRun(cmd.OutOrStdout(), flags, "instance list")
			}
			reg, err := fleet.Load()
			if err != nil {
				return configErr(err)
			}
			rows := make([]instanceListRow, 0, len(reg.Instances))
			for _, p := range reg.Instances {
				// Under --instance the other instances' secrets were scrubbed
				// from this process, so only the pinned entry can report secret_set.
				if pinnedInstance == "" || p.Name == pinnedInstance {
					rows = append(rows, instanceRow(p))
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

func newInstanceRemoveCmd(flags *rootFlags) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "remove <name>",
		Short: "Remove an instance from the registry and delete its cached token and local data",
		Example: strings.Trim(`
  jamf-pro-cli instance remove acme`, "\n"),
		Annotations: map[string]string{"pp:data-source": "local", "mcp:hidden": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 && cmd.Flags().NFlag() == 0 {
				return cmd.Help()
			}
			if dryRunOK(flags) {
				return writeDryRun(cmd.OutOrStdout(), flags, "instance remove")
			}
			if cliutil.IsAnyHarness() {
				return writeHarnessRefusal(cmd.OutOrStdout(), flags, "instance remove")
			}
			if len(args) != 1 {
				_ = cmd.Usage()
				return usageErr(fmt.Errorf("exactly one instance name is required"))
			}
			name := strings.TrimSpace(args[0])
			found, err := fleet.RemoveInstance(name)
			if err != nil {
				return configErr(err)
			}
			if !found {
				reg, _ := fleet.Load()
				names := []string{}
				if reg != nil {
					names = reg.Names()
				}
				return notFoundErr(fmt.Errorf("no instance named %q (registered: %s)", name, strings.Join(names, ", ")))
			}
			return printJSONFiltered(cmd.OutOrStdout(), map[string]any{"removed": name, "purged_local_data": true}, flags)
		},
	}
	return cmd
}
