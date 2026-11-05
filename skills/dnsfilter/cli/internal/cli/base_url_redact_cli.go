// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
//
// Hand-written (NOT generated): the CLI's error writers never show a URL's
// credentials. Pinned by this connector's handfixes.json under the
// "base-url-credentials-held-and-redacted" entry (class "secret-echo").

package cli

import (
	"github.com/spf13/cobra"

	"dnsfilter-pp-cli/internal/cliutil"
)

// redactCommandErrors makes every error a command returns show the URLs in
// it with their credentials redacted (cliutil.RedactURLCredentials), by
// wrapping each RunE-style hook in the tree once. Execute installs it before
// the kit's stderr check, so a held password yields the redacted text, not a
// withheld error.
func redactCommandErrors(root *cobra.Command) {
	var walk func(c *cobra.Command)
	walk = func(c *cobra.Command) {
		c.PersistentPreRunE = redactURLRunE(c.PersistentPreRunE)
		c.PreRunE = redactURLRunE(c.PreRunE)
		c.RunE = redactURLRunE(c.RunE)
		c.PostRunE = redactURLRunE(c.PostRunE)
		c.PersistentPostRunE = redactURLRunE(c.PersistentPostRunE)
		for _, sub := range c.Commands() {
			walk(sub)
		}
	}
	walk(root)
}

func redactURLRunE(fn func(*cobra.Command, []string) error) func(*cobra.Command, []string) error {
	if fn == nil {
		return nil
	}
	return func(cmd *cobra.Command, args []string) error {
		return redactURLError(fn(cmd, args))
	}
}

// urlRedactedError carries the redacted text and keeps the original error
// reachable through errors.As and errors.Is, so exit codes still classify.
type urlRedactedError struct {
	err error
	msg string
}

func (e urlRedactedError) Error() string { return e.msg }
func (e urlRedactedError) Unwrap() error { return e.err }

// redactURLError returns err, or an error whose text shows every URL in it
// with its credentials redacted when err's text carries one.
func redactURLError(err error) error {
	if err == nil {
		return nil
	}
	raw := err.Error()
	if msg := cliutil.RedactURLCredentials(raw); msg != raw {
		return urlRedactedError{err: err, msg: msg}
	}
	return err
}
