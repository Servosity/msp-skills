// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-written novel command (handfixes.json tracker-novels).
// pp:data-source live

package cli

import (
	"errors"
	"fmt"
	"sort"
	"strings"

	"domain-cert-tracker-pp-cli/internal/cliutil"
	"domain-cert-tracker-pp-cli/internal/tracker"

	"github.com/spf13/cobra"
)

func newNovelDnsCheckCmd(flags *rootFlags) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "check <domain>",
		Short: "Get NS, MX, SPF, DMARC, CAA and DNSSEC for a domain as one JSON document through the system resolver.",
		Long: `Use this command for the live NS, MX, SPF, DMARC, CAA and DNSSEC state of one
domain. Do NOT use this command to see what DNS changed since the last sweep;
use 'events list' instead.

Queries go to the resolver your operating system is configured with; the
domain argument never chooses the server. A lookup that fails (as opposed to
an empty answer) is listed under errors and the command exits non-zero.`,
		Example:     "  domain-cert-tracker-cli dns check example.com --agent",
		Annotations: map[string]string{"mcp:read-only": "true", "pp:data-source": "live", "pp:happy-args": "domain=example.com"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateDataSourceStrategy(flags, "live"); err != nil {
				return err
			}
			if len(args) == 0 && cmd.Flags().NFlag() == 0 {
				return cmd.Help()
			}
			if dryRunOK(flags) {
				return writeDryRun(cmd.OutOrStdout(), flags, "dns check")
			}
			if len(args) != 1 {
				return usageErr(errors.New("dns check needs exactly one domain"))
			}
			domain, err := tracker.NormalizeDomain(args[0])
			if err != nil {
				return usageErr(err)
			}
			if cliutil.IsVerifyEnv() {
				return writeDryRun(cmd.OutOrStdout(), flags, "dns check "+domain)
			}
			ctx, cancel := boundCtx(cmd.Context(), flags)
			defer cancel()
			obs := tracker.CheckDNS(ctx, &tracker.SystemDNS{}, domain)
			if err := flags.printJSON(cmd, obs); err != nil {
				return err
			}
			if !obs.OK() {
				keys := make([]string, 0, len(obs.Errors))
				for k := range obs.Errors {
					keys = append(keys, k)
				}
				sort.Strings(keys)
				return fmt.Errorf("dns check %s: %s lookups failed", domain, strings.Join(keys, ", "))
			}
			return nil
		},
	}
	return cmd
}
