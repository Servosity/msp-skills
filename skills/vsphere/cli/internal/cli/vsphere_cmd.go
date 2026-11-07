// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-written (not generated): scope validation and output shared by the
// novel commands. Each command declares its own flags (the SKILL/README flag
// checker reads them from the command file).

package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"vsphere-pp-cli/internal/client"
	"vsphere-pp-cli/internal/cliutil"
)

type scopeFlags struct {
	scope       invScope
	concurrency int
}

func (s *scopeFlags) validate() error {
	if s.scope.Cluster != "" && s.scope.Host != "" {
		return usageErr(fmt.Errorf("use --cluster or --host, not both"))
	}
	if s.concurrency < 1 {
		return usageErr(fmt.Errorf("--concurrency must be at least 1"))
	}
	if cliutil.IsDogfoodEnv() && s.concurrency > 4 {
		s.concurrency = 4
	}
	return nil
}

func nonNilFailures(f []fetchFailure) []fetchFailure {
	if f == nil {
		return make([]fetchFailure, 0)
	}
	return f
}

// requireVCenter stops before any network call when the vCenter address or
// the credentials are not configured, naming what to set.
func requireVCenter(c *client.Client) error {
	if c.BaseURL == "" || doctorBaseURLIsPlaceholder(c.BaseURL) {
		return configErr(fmt.Errorf("no vCenter configured: export VSPHERE_BASE_URL=https://<your-vcenter> (or set base_url in the config file)"))
	}
	if c.Config == nil || c.Config.VsphereUsername == "" || c.Config.VspherePassword == "" {
		return authErr(fmt.Errorf("no vCenter credentials: export VSPHERE_USERNAME=<user@domain> VSPHERE_PASSWORD=<password> (a Read-Only role is enough)"))
	}
	return nil
}

// vcenterClient builds the client the novel commands use. Their reads are
// live, so the response cache is off: nothing they read (licence keys
// included) is written to disk.
func vcenterClient(cmd *cobra.Command, flags *rootFlags) (*client.Client, error) {
	c, err := flags.newClient()
	if err != nil {
		return nil, err
	}
	c.NoCache = true
	if err := requireVCenter(c); err != nil {
		writeAPIErrorEnvelope(cmd.OutOrStdout(), flags, err, ExitCode(err))
		return nil, err
	}
	return c, nil
}
