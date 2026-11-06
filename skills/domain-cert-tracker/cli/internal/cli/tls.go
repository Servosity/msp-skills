// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-written novel parent (handfixes.json tracker-novels).
// pp:data-source auto
// Supported strategies: auto, local, live, or computed. Change this default deliberately.

package cli

import (
	"github.com/spf13/cobra"
)

func newNovelTlsCmd(flags *rootFlags) *cobra.Command {

	cmd := &cobra.Command{
		Use:         "tls",
		Short:       "Live TLS checks: probe the certificate a host serves",
		Example:     "  domain-cert-tracker-cli tls probe example.com --agent",
		Annotations: map[string]string{"mcp:read-only": "true", "pp:data-source": "auto", "pp:typed-exit-codes": "0,2"},
		RunE:        parentNoSubcommandRunE(flags),
	}
	addNovelCommandIfAbsent(cmd, newNovelTlsProbeCmd(flags))
	return cmd
}
