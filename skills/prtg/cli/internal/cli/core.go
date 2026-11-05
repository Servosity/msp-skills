// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-written: the core command group and the shared core list row.
// pp:data-source local

package cli

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"prtg-pp-cli/internal/fleet"
)

func init() {
	registerNovelCommand(func(root *cobra.Command, flags *rootFlags) {
		addNovelCommandIfAbsent(root, newCoreCmd(flags))
	})
}

func newCoreCmd(flags *rootFlags) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "core",
		Short: "Register the PRTG core servers you manage (one per client) for --core and fleet commands",
		Long: `Register the PRTG core servers you manage, usually one per client.

Each core gets a name and its host[:port]. Its API key comes from the one
PRTG_FLEET_KEYS environment variable, as name=key entries separated by commas
or newlines (e.g. acme=<key>,globex=<key>), and is never written to disk. Pass
--core <name> to any command to run it against that core, or use the fleet
commands to query every core at once.`,
		Example: strings.Trim(`
  prtg-cli core add acme --server prtg.acme.example:1616
  prtg-cli core list --json
  prtg-cli sensors list --core acme --json`, "\n"),
		Annotations: map[string]string{"pp:data-source": "local", "pp:parent-group": "true", "mcp:read-only": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
	}
	cmd.AddCommand(newCoreAddCmd(flags), newCoreListCmd(flags), newCoreRemoveCmd(flags))
	return cmd
}

type coreListRow struct {
	Name   string `json:"name"`
	Server string `json:"server"`
	// KeySet is true when PRTG_FLEET_KEYS has a non-empty entry for the core.
	KeySet      bool     `json:"key_set"`
	InsecureTLS bool     `json:"insecure_tls"`
	Tags        []string `json:"tags"`
}

// coreRow describes c; keys is the parsed PRTG_FLEET_KEYS (nil when it does
// not parse, so every core reads as key_set false).
func coreRow(c fleet.Core, keys map[string]string) coreListRow {
	tags := c.Tags
	if tags == nil {
		tags = []string{}
	}
	return coreListRow{Name: c.Name, Server: c.Server, KeySet: keys[c.Name] != "", InsecureTLS: c.InsecureTLS, Tags: tags}
}

// fleetKeysOrWarn parses PRTG_FLEET_KEYS for a listing; a value that does not
// parse is reported on stderr (the error never repeats key text) and lists
// every core without a key.
func fleetKeysOrWarn(cmd *cobra.Command) map[string]string {
	keys, err := fleet.Keys()
	if err != nil {
		fmt.Fprintf(cmd.ErrOrStderr(), "warning: %v\n", err)
	}
	return keys
}

// hintEmptyFleet tells a human how to register the first core.
func hintEmptyFleet(cmd *cobra.Command) {
	fmt.Fprintf(cmd.ErrOrStderr(), "hint: no PRTG cores registered, so nothing was checked; add one with: %s core add <name> --server <host[:port]>\n", cmd.Root().Name())
}
