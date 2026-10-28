// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

// kaseya-vsa handfix (output-chokepoint): Execute installs the chokepoint
// (cliutil.InstallOutputFunnel) before the command runs and waits for it to
// deliver everything before the process exits.

import (
	"encoding/base64"
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"kaseya-vsa-pp-cli/internal/cliutil"
)

func installOutputChokepoint(rootCmd *cobra.Command) func() {
	holdEnvCredentials()
	restore, outTTY, errTTY, err := cliutil.InstallOutputFunnel()
	if err != nil {
		// Fail closed: no command runs without the chokepoint.
		fmt.Fprintln(os.Stderr, "error: cannot start the output filter:", err)
		os.Exit(1)
	}
	rootCmd.SetOut(cliutil.TTYWriter{W: os.Stdout, TTY: outTTY})
	rootCmd.SetErr(cliutil.TTYWriter{W: os.Stderr, TTY: errTTY})
	return restore
}

// holdEnvCredentials registers every credential the environment supplies,
// before any output; configuration and clients register theirs when they load.
func holdEnvCredentials() {
	id := cliutil.EnvOverride("KASEYA_VSA_TOKEN_ID")
	secret := cliutil.EnvOverride("KASEYA_VSA_TOKEN_SECRET")
	cliutil.FunnelHold(secret, os.Getenv("KASEYA_VSA_MCP_HTTP_TOKEN"))
	if id != "" && secret != "" {
		cliutil.FunnelHold("Basic " + base64.StdEncoding.EncodeToString([]byte(id+":"+secret)))
	}
}
