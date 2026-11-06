// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-written novel parent (handfixes.json tracker-novels).
// pp:data-source auto
// Supported strategies: auto, local, live, or computed. Change this default deliberately.

package cli

import (
	"github.com/spf13/cobra"
)

func newNovelDnsCmd(flags *rootFlags) *cobra.Command {

	cmd := &cobra.Command{
		Use:         "dns",
		Short:       "Live DNS checks: NS, MX, SPF, DMARC, CAA and DNSSEC",
		Example:     "  domain-cert-tracker-cli dns check example.com --agent",
		Annotations: map[string]string{"mcp:read-only": "true", "pp:data-source": "auto", "pp:typed-exit-codes": "0,2"},
		RunE:        parentNoSubcommandRunE(flags),
	}
	addNovelCommandIfAbsent(cmd, newNovelDnsCheckCmd(flags))
	return cmd
}
