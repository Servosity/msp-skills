// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"encoding/json"
	"errors"
	"io"
	"os"

	"github.com/spf13/cobra"

	"jamf-pro-pp-cli/internal/cliutil"
)

// ownReport marks output that is this program's own status text, not data read
// from Jamf Pro: doctor's "credentials" row is a verdict, and judging it as a
// credential would hold the verdict and withhold the whole report. The kit still
// withholds anything that repeats a held credential.
var ownReport bool

// printOwnReport prints one report under ownReport.
func printOwnReport(w io.Writer, report any, flags *rootFlags) error {
	ownReport = true
	defer func() { ownReport = false }()
	return printJSONFiltered(w, report, flags)
}

// shownData and shownRows apply the one credential rule to data a generated
// formatter is about to print (jamf-pro handfix one-rule-everywhere; hooks in
// helpers.go printOutput, printAutoTable, printCSV, printPlain, printQuiet):
// every output format (JSON, table, cards, CSV, plain, quiet) shows what the
// rule leaves, whichever command printed it. Values the rule finds are held, so
// the credential kit withholds a repeat elsewhere.
func shownData(data json.RawMessage) json.RawMessage {
	if ownReport {
		return data
	}
	return json.RawMessage(cliutil.RedactText(string(data)))
}

func shownRows(items []map[string]any) []map[string]any {
	for i, item := range items {
		if r, ok := cliutil.RedactShown(item).(map[string]any); ok {
			items[i] = r
		}
	}
	return items
}

// refuseMalformedStdinBody stops a --stdin request body that does not parse with
// fixed text, before a generated command's JSON parser quotes a byte of it
// (handfixes.json stdin-body-never-quoted). The body stays readable for the
// command. 'feedback --stdin' takes free text and has no endpoint annotation.
func refuseMalformedStdinBody(cmd *cobra.Command) error {
	if cmd.Annotations["pp:endpoint"] == "" {
		return nil
	}
	if on, err := cmd.Flags().GetBool("stdin"); err != nil || !on {
		return nil
	}
	data, err := io.ReadAll(os.Stdin)
	if err != nil {
		return usageErr(errors.New("the --stdin body could not be read"))
	}
	if !json.Valid(data) {
		return usageErr(errors.New("the --stdin body is not valid JSON"))
	}
	r, w, err := os.Pipe()
	if err != nil {
		return err
	}
	go func() {
		_, _ = w.Write(data)
		_ = w.Close()
	}()
	os.Stdin = r
	return nil
}
