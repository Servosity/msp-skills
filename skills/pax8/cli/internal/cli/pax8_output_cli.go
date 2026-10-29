// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

package cli

// pax8 handfix (output-chokepoint): Execute installs the chokepoint
// (cliutil.InstallOutputFunnel) first, before the command line is parsed and
// before any startup source loads; the funnel then holds every credential the
// environment, the stored configuration and the command line carry and only
// after that delivers a line (a configuration warning printed while it loads
// included: Codex r1 class output-before-chokepoint). It waits for the
// chokepoint to deliver everything before the process exits. Clients and
// token mints hold theirs when they answer.

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	"pax8-pp-cli/internal/cliutil"
	"pax8-pp-cli/internal/config"
	"pax8-pp-cli/internal/learn"
)

func installOutputChokepoint(rootCmd *cobra.Command) func() {
	restore, outTTY, errTTY, err := cliutil.InstallOutputFunnel(func() {
		cliutil.HoldEnvCredentials()
		holdStoredCredentials()
		holdArgvCredentials(rootCmd, os.Args[1:])
	})
	if err != nil {
		// Fail closed: no command runs without the chokepoint.
		fmt.Fprintln(os.Stderr, "error: cannot start the output filter:", err)
		os.Exit(1)
	}
	rootCmd.SetOut(cliutil.TTYWriter{W: os.Stdout, TTY: outTTY})
	rootCmd.SetErr(cliutil.TTYWriter{W: os.Stderr, TTY: errTTY})
	return restore
}

// holdStoredCredentials loads the configuration (the config file named by
// PAX8_CONFIG or the default path, and the credential store) only to
// hold what it carries (config.Load holds it), so a flag error or an early
// message cannot print a stored credential either. A run under --home or
// --config holds that configuration when the command loads it; the command
// line is not read here (an os.Args path would reach a file open, which the
// MCP file-flag gate refuses). Errors are left to the command.
func holdStoredCredentials() {
	_, _ = config.Load("")
}

// argvCredentialFlag reports a flag whose value is a credential: every name
// cliutil.SecretFieldName matches (--client-secret, --password, --psk,
// --token, ...) and --client-id, the public half of a client credential pair.
func argvCredentialFlag(name string) bool {
	return cliutil.SecretFieldName(name) || strings.EqualFold(name, "client-id")
}

// holdArgvCredentials holds the value of every credential flag, as
// --name=value or --name value, so a flag error or a preview cannot print
// them. A non-boolean flag takes the next argument whatever it starts with,
// so that argument is held too. ('auth set-token' reads its token from stdin
// and holds it there.)
func holdArgvCredentials(root *cobra.Command, args []string) {
	cmd, _, _ := root.Find(args)
	for i, a := range args {
		if a == "--" {
			return
		}
		name, value, hasValue := strings.Cut(strings.TrimLeft(a, "-"), "=")
		if !strings.HasPrefix(a, "-") || !argvCredentialFlag(name) {
			continue
		}
		if hasValue {
			cliutil.HoldKeys(value)
		} else if i+1 < len(args) && !isBoolFlag(cmd, name) {
			cliutil.HoldKeys(args[i+1])
		}
	}
}

// isBoolFlag reports whether cmd or an ancestor declares name as a boolean
// flag (it then takes no separate value).
func isBoolFlag(cmd *cobra.Command, name string) bool {
	for c := cmd; c != nil; c = c.Parent() {
		for _, fs := range []*pflag.FlagSet{c.Flags(), c.PersistentFlags()} {
			if f := fs.Lookup(name); f != nil {
				return f.Value.Type() == "bool"
			}
		}
	}
	return false
}

// withheldJournalEntry passes every text field of a learn journal entry
// through the chokepoint before the journal writes it (pax8 handfix:
// output-chokepoint, file sinks); the generated journal writer stays
// byte-identical to the fleet.
func withheldJournalEntry(e learn.JournalEntry) learn.JournalEntry {
	e.Cmd = withheldStrings(e.Cmd)
	shape := make(map[string]string, len(e.ArgvShape))
	for k, v := range e.ArgvShape {
		shape[cliutil.WithholdText(k)] = cliutil.WithholdText(v)
	}
	e.ArgvShape = shape
	e.ErrorClass = cliutil.WithholdText(e.ErrorClass)
	e.FailedFlag = cliutil.WithholdText(e.FailedFlag)
	e.SuggestedFlag = cliutil.WithholdText(e.SuggestedFlag)
	e.QueryFamily = cliutil.WithholdText(e.QueryFamily)
	e.UnresolvedEntities = withheldStrings(e.UnresolvedEntities)
	return e
}

func withheldStrings(in []string) []string {
	if in == nil {
		return nil
	}
	out := make([]string, len(in))
	for i, s := range in {
		out[i] = cliutil.WithholdText(s)
	}
	return out
}
