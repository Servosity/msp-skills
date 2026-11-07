// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

// Pre-run checks for the generated `seats bulk-change` command so --dry-run
// previews only requests that would be sent. The assembled payload goes
// through the same canonical validator the transport guard uses
// (bulk_seat_guard.go), with ids parsed by the generated list parser, so a
// preview and a real send accept exactly the same input.

package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"datto-saas-pp-cli/internal/cliutil"
)

func init() {
	registerNovelCommand(func(root *cobra.Command, flags *rootFlags) {
		bc, _, err := root.Find([]string{"seats", "bulk-change"})
		if err != nil || bc == nil || bc.Name() != "bulk-change" {
			return
		}
		// The generated help truncates the spec text at 120 characters, which
		// drops the write warning, the Unlicense action and the 100-id limit.
		bc.Long = bulkChangeLong
		if f := bc.Flags().Lookup("action-type"); f != nil {
			f.Usage = "License protects the seat (billable), Pause keeps existing backups but stops new ones, Unlicense stops protecting the seat. One of: License, Pause, Unlicense (case-sensitive)."
		}
		if f := bc.Flags().Lookup("ids"); f != nil {
			f.Usage = "Seat remoteId values from 'seats list', comma-separated or a JSON array. At most 100 per call."
		}
		prev := bc.PreRunE
		bc.PreRunE = func(cmd *cobra.Command, args []string) error {
			if err := checkBulkChangeInput(cmd, flags); err != nil {
				return err
			}
			if prev != nil {
				return prev(cmd, args)
			}
			return nil
		}
	})
}

const bulkChangeLong = `License, pause, or unlicense up to 100 seats of one subscription in one call.

This is the only write: it changes which seats are protected and billed, and it
sends immediately with no confirmation prompt (--yes is not required). Preview
with --dry-run, which applies the same seat-type, action and 100-id checks as a
real send, or plan batches with 'seats bulk-plan'.

Take saasCustomerId and externalSubscriptionId from 'domains', and the ids from
the remoteId column of 'seats list'. Google Workspace tenants must be on Seat
Management 2.0. The request is never retried automatically, because a 5xx may
already have been applied, and --idempotent is refused.`

func checkBulkChangeInput(cmd *cobra.Command, flags *rootFlags) error {
	if flags.idempotent {
		return usageErr(fmt.Errorf("--idempotent is not supported for seats bulk-change: it would report an HTTP 409 on a billing change as success"))
	}
	// The transport guard refuses these on send; refuse them in preview too.
	if c, err := flags.newClient(); err == nil && c != nil && c.Config != nil {
		for name, value := range c.Config.Headers {
			if (strings.EqualFold(name, "Idempotency-Key") || strings.EqualFold(name, "X-Idempotency-Key")) && strings.TrimSpace(value) != "" {
				return usageErr(fmt.Errorf("remove the configured %s header: Datto documents no idempotency for bulkSeatChange, so it must never be replayed", name))
			}
		}
	}
	if stdin, _ := cmd.Flags().GetBool("stdin"); stdin {
		// The generated command reads os.Stdin itself: read it here, validate,
		// and hand the same bytes back through a pipe.
		data, err := cliutil.ReadRequestBody(os.Stdin) // Hand-fix (handfixes.json credkit-wiring): held before a preview or a send
		if err != nil {
			return fmt.Errorf("reading stdin: %w", err)
		}
		canonical, err := canonicalBulkSeatBody(data)
		if err != nil {
			return usageErr(err)
		}
		// Re-pipe the canonical bytes so the dry-run preview equals what is sent.
		r, w, err := os.Pipe()
		if err != nil {
			return err
		}
		go func() { _, _ = w.Write(canonical); _ = w.Close() }()
		os.Stdin = r
		return nil
	}
	// Leave wholly missing flags to the generated "required flag" errors.
	if !cmd.Flags().Changed("seat-type") || !cmd.Flags().Changed("action-type") || !cmd.Flags().Changed("ids") {
		return nil
	}
	seatType, _ := cmd.Flags().GetString("seat-type")
	actionType, _ := cmd.Flags().GetString("action-type")
	rawIDs, _ := cmd.Flags().GetString("ids")
	ids, err := cliutil.ParseStringList(rawIDs)
	if err != nil {
		return usageErr(fmt.Errorf("--ids: %w", err))
	}
	body, err := json.Marshal(map[string]any{"seat_type": seatType, "action_type": actionType, "ids": ids})
	if err != nil {
		return err
	}
	if _, err := canonicalBulkSeatBody(body); err != nil {
		return usageErr(err)
	}
	return nil
}
