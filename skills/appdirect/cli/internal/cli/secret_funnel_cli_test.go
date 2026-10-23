// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
//
// Hand-written (NOT generated): tests for the CLI's error backstop
// (secret_funnel_cli.go). Pinned by this connector's handfixes.json under the
// "secret-echo-decoded-funnel" entry (class "secret-echo").

package cli

import (
	"bytes"
	"encoding/base64"
	"errors"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"appdirect-pp-cli/internal/cliutil"
)

func TestFunnelCommandErrorsWithholdsAnEchoedCredential(t *testing.T) {
	held := "cli-backstop-held-0a1+2b3/4c5="
	cliutil.FunnelHold(held)
	echo := base64.StdEncoding.EncodeToString([]byte(held))
	leaky := errors.New("server said " + echo)
	root := &cobra.Command{Use: "root", SilenceUsage: true}
	root.AddCommand(&cobra.Command{Use: "leaky", RunE: func(*cobra.Command, []string) error { return leaky }})
	root.AddCommand(&cobra.Command{Use: "clean", RunE: func(*cobra.Command, []string) error { return errors.New("plain failure") }})
	funnelCommandErrors(root)
	var printed bytes.Buffer
	root.SetErr(&printed)
	root.SetOut(&printed)

	root.SetArgs([]string{"leaky"})
	err := root.Execute()
	if err == nil || strings.Contains(err.Error(), echo) || strings.Contains(printed.String(), echo) {
		t.Fatalf("an echoed credential reached the error (%v) or the printed text (%q)", err, printed.String())
	}
	if !cliutil.FunnelWithheld(err.Error()) || !errors.Is(err, leaky) {
		t.Fatalf("the error must carry the withheld notice and keep the original reachable: %v", err)
	}
	if got := funnelShownError(leaky); strings.Contains(got, echo) {
		t.Fatalf("the error envelope text carried the echo: %q", got)
	}

	printed.Reset()
	root.SetArgs([]string{"clean"})
	if err := root.Execute(); err == nil || err.Error() != "plain failure" {
		t.Fatalf("an error with no credential in it was changed: %v", err)
	}
	if got := funnelShownError(errors.New("plain failure")); got != "plain failure" {
		t.Fatalf("clean envelope text was changed: %q", got)
	}
}
