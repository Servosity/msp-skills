// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
//
// Hand-written (NOT generated): no username or password from SUPEROPS_BASE_URL
// reaches a command's printed error (superops-base-url-never-printed). Kept out
// of secret_funnel_cli.go, which ships byte-identical in every connector.

package cli

import (
	"github.com/spf13/cobra"

	"superops-pp-cli/internal/cliutil"
)

// redactCommandErrorURLUserinfo wraps every command's run functions, outside
// the shared funnel's wrap, so an error whose text carries a URL's userinfo
// (net/http's url.Error keeps the username) prints without it.
func redactCommandErrorURLUserinfo(root *cobra.Command) {
	var walk func(*cobra.Command)
	walk = func(c *cobra.Command) {
		c.PersistentPreRunE = redactRunEURLUserinfo(c.PersistentPreRunE)
		c.PreRunE = redactRunEURLUserinfo(c.PreRunE)
		c.RunE = redactRunEURLUserinfo(c.RunE)
		c.PostRunE = redactRunEURLUserinfo(c.PostRunE)
		c.PersistentPostRunE = redactRunEURLUserinfo(c.PersistentPostRunE)
		for _, sub := range c.Commands() {
			walk(sub)
		}
	}
	walk(root)
}

func redactRunEURLUserinfo(fn func(*cobra.Command, []string) error) func(*cobra.Command, []string) error {
	if fn == nil {
		return nil
	}
	return func(cmd *cobra.Command, args []string) error {
		return redactErrorURLUserinfo(fn(cmd, args))
	}
}

// urlUserinfoRedactedError shows err's text without URL userinfo and keeps err
// reachable through errors.As and errors.Is, so exit codes still classify.
type urlUserinfoRedactedError struct {
	err error
	msg string
}

func (e urlUserinfoRedactedError) Error() string { return e.msg }
func (e urlUserinfoRedactedError) Unwrap() error { return e.err }

func redactErrorURLUserinfo(err error) error {
	if err == nil {
		return nil
	}
	if msg := cliutil.RedactURLUserinfo(err.Error()); msg != err.Error() {
		return urlUserinfoRedactedError{err: err, msg: msg}
	}
	return err
}
