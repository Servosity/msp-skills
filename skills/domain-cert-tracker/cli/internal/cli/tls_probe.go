// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-written novel command (handfixes.json tracker-novels).
// pp:data-source live
// pp:client-call (a live TLS handshake through tracker.TLSProber and the netguard dialer, not an HTTP API)

package cli

import (
	"errors"
	"fmt"

	"domain-cert-tracker-pp-cli/internal/cliutil"
	"domain-cert-tracker-pp-cli/internal/tracker"

	"github.com/spf13/cobra"
)

func newNovelTlsProbeCmd(flags *rootFlags) *cobra.Command {
	var allowPorts []int
	cmd := &cobra.Command{
		Use:   "probe <host[:port]>",
		Short: "Handshake with a live host and get its chain, expiry, SANs, key, issuer, hostname match and OCSP must-staple as JSON.",
		Long: `Use this command to inspect the certificate one host is serving right now.
Do NOT use this command to find renewals that were issued but never deployed
across the estate; use 'renewals' instead.

The host must resolve only to public addresses, and the port must be 443
unless you pass --allow-port <port> on the command line (MCP calls cannot).
Expired, self-signed and wrong-host certificates are reported, not rejected.`,
		Example:     "  domain-cert-tracker-cli tls probe example.com --agent",
		Annotations: map[string]string{"mcp:read-only": "true", "pp:data-source": "live", "pp:happy-args": "host=example.com", "mcp:read-flags": "allow-port"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateDataSourceStrategy(flags, "live"); err != nil {
				return err
			}
			if len(args) == 0 && cmd.Flags().NFlag() == 0 {
				return cmd.Help()
			}
			if dryRunOK(flags) {
				return writeDryRun(cmd.OutOrStdout(), flags, "tls probe")
			}
			if len(args) != 1 {
				return usageErr(errors.New("tls probe needs exactly one host[:port]"))
			}
			host, port, err := parseHostTarget(args[0])
			if err != nil {
				return usageErr(err)
			}
			g, err := trackerGuard(allowPorts)
			if err != nil {
				return err
			}
			if err := g.CheckPort(port); err != nil {
				return usageErr(err)
			}
			if cliutil.IsVerifyEnv() {
				return writeDryRun(cmd.OutOrStdout(), flags, "tls probe "+args[0])
			}
			ctx, cancel := boundCtx(cmd.Context(), flags)
			defer cancel()
			obs, err := (&tracker.TLSProber{Guard: g}).Probe(ctx, host, port)
			if err != nil {
				return fmt.Errorf("tls probe %s: %w", args[0], err)
			}
			return flags.printJSON(cmd, obs)
		},
	}
	// mcp:read-flags drops --allow-port from the MCP schema; trackerGuard
	// also refuses it at run time under the MCP server.
	cmd.Flags().IntSliceVar(&allowPorts, "allow-port", nil, "Also allow this TCP port for this run (command line only; default is 443 only)")
	return cmd
}
