// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"cisco-umbrella-pp-cli/internal/client"
	"cisco-umbrella-pp-cli/internal/config"
)

// --org scopes a run to one child (customer) org of a Multi-org or MSP console:
// the token request carries X-Umbrella-OrgId, and the local store is per org.
func init() {
	registerNovelCommand(func(root *cobra.Command, flags *rootFlags) {
		if root.PersistentFlags().Lookup("org") != nil {
			return
		}
		var org string
		root.PersistentFlags().StringVar(&org, "org", "", "Child org ID to act on from a Multi-org/MSP console key (env "+config.OrgEnv+"; list IDs with 'managed-customers list')")
		prev := root.PersistentPreRunE
		root.PersistentPreRunE = func(cmd *cobra.Command, args []string) error {
			if err := exportOrg(org); err != nil {
				return err
			}
			if prev != nil {
				if err := prev(cmd, args); err != nil {
					return err
				}
			}
			// The generated hook may have applied a saved --profile that sets org,
			// so export again and re-derive the store scope for the final org. The
			// legacy (token-named) store is never adopted into a child org's scope.
			if err := exportOrg(org); err != nil {
				return err
			}
			if config.ChildOrgID() != "" {
				cfg, err := config.Load(flags.configPath)
				if err != nil {
					return configErr(err)
				}
				setDefaultDBScopeIdentity(cfg.StoreScopeCredential(), "", cfg.BaseURL)
			}
			return nil
		}
	})
	registerClientHook(func(c *client.Client) error {
		// Clients are built after PersistentPreRunE, so the env holds the final org.
		id := config.ChildOrgID()
		if id == "" {
			return nil
		}
		if !config.ValidOrgID(id) {
			return usageErr(fmt.Errorf("%s must be a numeric Umbrella organization ID, got %q", config.OrgEnv, id))
		}
		client.ScopeToChildOrg(c, id)
		return nil
	})
}

// exportOrg makes --org (flag or profile) and CISCO_UMBRELLA_ORG_ID one source of
// truth, and rejects anything but a numeric org ID before it reaches a header.
func exportOrg(org string) error {
	if org != "" {
		if err := os.Setenv(config.OrgEnv, org); err != nil {
			return err
		}
	}
	if id := config.ChildOrgID(); id != "" && !config.ValidOrgID(id) {
		return usageErr(fmt.Errorf("--org / %s must be a numeric Umbrella organization ID, got %q", config.OrgEnv, id))
	}
	return nil
}
