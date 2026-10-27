// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
//
// Hand-written (NOT generated), cove only: the CLI half of the
// short-credential backstop. The shared backstop (secret_funnel_cli.go,
// byte-identical across connectors and left untouched) looks only for held
// credentials of at least 8 characters; this file applies
// cliutil.FunnelHeldAll, which also withholds a shorter held credential (a
// short COVE_PASSWORD API token), over the same surfaces: every error a
// command returns (Execute wires coveShortFunnelCommandErrors next to
// funnelCommandErrors) and the JSON error envelopes (helpers.go passes the
// error through coveShortFunnelError before funnelShownError). Hand-fix code:
// cove-short-credential-funnel.

package cli

import (
	"github.com/spf13/cobra"

	"cove-pp-cli/internal/cliutil"
)

// coveShortFunnelCommandErrors wraps every RunE-style hook in the tree once,
// so an error a command returns passes cliutil.FunnelHeldAll before Cobra
// prints it. Wired after funnelCommandErrors, it wraps the shared backstop's
// wrappers, so both run on every error. Success output is never funnelled.
func coveShortFunnelCommandErrors(root *cobra.Command) {
	var walk func(c *cobra.Command)
	walk = func(c *cobra.Command) {
		c.PersistentPreRunE = coveShortFunnelRunE(c.PersistentPreRunE)
		c.PreRunE = coveShortFunnelRunE(c.PreRunE)
		c.RunE = coveShortFunnelRunE(c.RunE)
		c.PostRunE = coveShortFunnelRunE(c.PostRunE)
		c.PersistentPostRunE = coveShortFunnelRunE(c.PersistentPostRunE)
		for _, sub := range c.Commands() {
			walk(sub)
		}
	}
	walk(root)
}

func coveShortFunnelRunE(fn func(*cobra.Command, []string) error) func(*cobra.Command, []string) error {
	if fn == nil {
		return nil
	}
	return func(cmd *cobra.Command, args []string) error {
		return coveShortFunnelError(fn(cmd, args))
	}
}

// coveShortFunnelledError carries the withheld text and keeps the original
// error reachable through errors.As and errors.Is, so exit codes still
// classify.
type coveShortFunnelledError struct {
	err error
	msg string
}

func (e coveShortFunnelledError) Error() string { return e.msg }
func (e coveShortFunnelledError) Unwrap() error { return e.err }

// coveShortFunnelError returns err, or an error with the funnel's withheld
// notice when err's text repeats a credential this process holds, short ones
// included.
func coveShortFunnelError(err error) error {
	if err == nil {
		return nil
	}
	if msg := cliutil.FunnelHeldAll(err.Error()); cliutil.FunnelWithheld(msg) {
		return coveShortFunnelledError{err: err, msg: msg}
	}
	return err
}
