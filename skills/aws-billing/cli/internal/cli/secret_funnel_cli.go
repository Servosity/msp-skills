// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
//
// Hand-written (NOT generated): the CLI's error backstop for the shared
// secret-echo funnel. Pinned by this connector's handfixes.json under the
// "secret-echo-decoded-funnel" entry (class "secret-echo").

package cli

import (
	"github.com/spf13/cobra"

	"aws-billing-pp-cli/internal/cliutil"
)

// funnelCommandErrors makes every error a command returns pass the shared
// funnel (cliutil.FunnelHeld) before Cobra prints it, by wrapping each
// RunE-style hook in the tree once. Success output is never funnelled.
func funnelCommandErrors(root *cobra.Command) {
	var walk func(c *cobra.Command)
	walk = func(c *cobra.Command) {
		c.PersistentPreRunE = funnelRunE(c.PersistentPreRunE)
		c.PreRunE = funnelRunE(c.PreRunE)
		c.RunE = funnelRunE(c.RunE)
		c.PostRunE = funnelRunE(c.PostRunE)
		c.PersistentPostRunE = funnelRunE(c.PersistentPostRunE)
		for _, sub := range c.Commands() {
			walk(sub)
		}
	}
	walk(root)
}

func funnelRunE(fn func(*cobra.Command, []string) error) func(*cobra.Command, []string) error {
	if fn == nil {
		return nil
	}
	return func(cmd *cobra.Command, args []string) error {
		return funnelError(fn(cmd, args))
	}
}

// funnelledError carries the withheld text and keeps the original error
// reachable through errors.As and errors.Is, so exit codes still classify.
type funnelledError struct {
	err error
	msg string
}

func (e funnelledError) Error() string { return e.msg }
func (e funnelledError) Unwrap() error { return e.err }

// funnelError returns err, or an error with the funnel's withheld notice when
// err's text repeats a credential this process holds.
func funnelError(err error) error {
	if err == nil {
		return nil
	}
	if msg := cliutil.FunnelHeld(err.Error()); cliutil.FunnelWithheld(msg) {
		return funnelledError{err: err, msg: msg}
	}
	return err
}

// funnelShownError is err.Error() for an error envelope, funnelled.
func funnelShownError(err error) string {
	return cliutil.FunnelHeld(err.Error())
}
