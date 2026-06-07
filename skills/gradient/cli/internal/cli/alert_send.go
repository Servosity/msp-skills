// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-authored novel command (Phase 3); survives regeneration as a whole file.

// pp:data-source live

package cli

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"gradient-pp-cli/internal/cliutil"
	"gradient-pp-cli/internal/ledger"
)

type alertSendView struct {
	MessageID    string `json:"message_id"`
	AlertID      string `json:"alert_id"`
	AccountID    string `json:"account_id"`
	TicketID     string `json:"ticket_id,omitempty"`
	TicketStatus string `json:"ticket_status"`
	Waited       string `json:"waited,omitempty"`
	Note         string `json:"note,omitempty"`
}

func newNovelAlertSendCmd(flags *rootFlags) *cobra.Command {
	var (
		flagAccount     string
		flagTitle       string
		flagDescription string
		flagAlertID     string
		flagPriority    float64
		flagStatus      float64
		flagDue         string
		flagService     string
		flagWait        bool
		flagTimeout     time.Duration
		flagInterval    time.Duration
	)

	cmd := &cobra.Command{
		Use:   "send",
		Short: "Dispatch an alert and (optionally) wait for its PSA ticket",
		Long: strings.Trim(`
Use this command to dispatch an alert and wait for its PSA ticket. Do NOT use
this command to re-check an alert you already dispatched; use 'alert trace'.
Do NOT use it to inspect a raw debug response for a known messageId; use
'ticket-events'.

Dispatch is asynchronous upstream: the API queues the alert and returns a
messageId; the PSA ticket is created later. With --wait this command polls
the ticket-event debug route until the ticket exists or --timeout-wait
elapses. Every dispatch (and its outcome) is recorded in the local alert
ledger that powers 'alert trace'.
`, "\n"),
		Example: strings.Trim(`
  gradient-cli alert send --account 123456789 --title "Backup failure" --description "Nightly job failed twice"
  gradient-cli alert send --account 123456789 --title "Backup failure" --description "Nightly job failed twice" --wait
  gradient-cli alert send --account 123456789 --title "Disk 90%" --description "Volume nearly full" --priority 3 --wait --timeout-wait 5m`, "\n"),
		Annotations: map[string]string{
			"mcp:read-only":       "false",
			"pp:typed-exit-codes": "0,1,2",
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 && cmd.Flags().NFlag() == 0 {
				return cmd.Help()
			}
			if len(args) > 0 {
				_ = cmd.Usage()
				return usageErr(fmt.Errorf("alert send takes no positional arguments; use --account/--title/--description flags"))
			}
			if dryRunOK(flags) {
				fmt.Fprintln(cmd.OutOrStdout(), "would dispatch the alert to the queue, record its messageId in the local ledger, and (with --wait) poll until the PSA ticket exists")
				return nil
			}
			var missing []string
			if flagAccount == "" {
				missing = append(missing, "--account")
			}
			if flagTitle == "" {
				missing = append(missing, "--title")
			}
			if flagDescription == "" {
				missing = append(missing, "--description")
			}
			if len(missing) > 0 {
				_ = cmd.Usage()
				return usageErr(fmt.Errorf("required flag(s) %s not set", strings.Join(missing, ", ")))
			}
			if flags.dataSource == "local" {
				return fmt.Errorf("alert send has no local data source: it dispatches to the live alerting queue")
			}

			c, err := flags.newClient()
			if err != nil {
				return err
			}

			alertID := flagAlertID
			if alertID == "" {
				buf := make([]byte, 8)
				if _, err := rand.Read(buf); err != nil {
					return fmt.Errorf("generating alert id: %w", err)
				}
				alertID = "cli-" + hex.EncodeToString(buf)
			}

			body := map[string]any{
				"title":       flagTitle,
				"description": flagDescription,
				"alertId":     alertID,
				"priority":    flagPriority,
				"status":      flagStatus,
			}
			if flagDue != "" {
				body["dueDateTime"] = flagDue
			}
			if flagService != "" {
				body["serviceId"] = flagService
			}

			path := replacePathParam("/vendor-api/alerting/{accountId}", "accountId", flagAccount)
			data, _, err := c.Post(cmd.Context(), path, body)
			if err != nil {
				return classifyAPIError(fmt.Errorf("dispatching alert: %w", err), flags)
			}
			var created struct {
				MessageID string `json:"messageId"`
				AlertID   string `json:"alertId"`
				Data      *struct {
					MessageID string `json:"messageId"`
					AlertID   string `json:"alertId"`
				} `json:"data"`
			}
			if err := json.Unmarshal(data, &created); err != nil {
				return fmt.Errorf("parsing dispatch response: %w", err)
			}
			if created.MessageID == "" && created.Data != nil {
				created.MessageID = created.Data.MessageID
				created.AlertID = created.Data.AlertID
			}
			if created.AlertID == "" {
				created.AlertID = alertID
			}

			rec := ledger.AlertRecord{
				At:           time.Now().UTC(),
				AccountID:    flagAccount,
				AlertID:      created.AlertID,
				MessageID:    created.MessageID,
				Title:        flagTitle,
				TicketStatus: "pending",
			}
			ledgerOK := true
			dir, lerr := ledger.Dir()
			if lerr != nil {
				ledgerOK = false
				fmt.Fprintf(cmd.ErrOrStderr(), "warning: alert dispatched but ledger unavailable: %v\n", lerr)
			} else if err := ledger.AppendAlert(dir, rec); err != nil {
				ledgerOK = false
				fmt.Fprintf(cmd.ErrOrStderr(), "warning: alert dispatched but ledger write failed: %v\n", err)
			}

			view := alertSendView{
				MessageID:    created.MessageID,
				AlertID:      created.AlertID,
				AccountID:    flagAccount,
				TicketStatus: "pending",
			}
			if created.MessageID == "" {
				view.Note = "dispatch succeeded but no messageId returned; cannot poll for the ticket"
				return printJSONFiltered(cmd.OutOrStdout(), view, flags)
			}
			if !flagWait {
				view.Note = "dispatched (async): re-check later with 'alert trace' or 'ticket-events " + created.MessageID + "'"
				return printJSONFiltered(cmd.OutOrStdout(), view, flags)
			}

			// Poll the ticket-event debug route until the ticket exists or timeout.
			timeout := flagTimeout
			interval := flagInterval
			if interval < time.Second {
				// Floor the poll pace so --interval 0 cannot hammer the
				// debug route in a tight spin for the whole wait window.
				interval = time.Second
			}
			if cliutil.IsDogfoodEnv() {
				// Live-dogfood matrix runs under a flat per-command timeout;
				// one poll round is enough to prove the loop.
				timeout = interval
			}
			start := time.Now()
			deadline := start.Add(timeout)
			debugPath := replacePathParam("/vendor-api/alerting/debug/{messageId}", "messageId", created.MessageID)
			for {
				dbg, err := c.GetNoCache(cmd.Context(), debugPath, nil)
				if err == nil {
					var ticket struct {
						TicketID string `json:"ticketId"`
						Status   string `json:"status"`
					}
					if json.Unmarshal(dbg, &ticket) == nil && ticket.TicketID != "" {
						view.TicketID = ticket.TicketID
						view.TicketStatus = "created"
						break
					}
				}
				if time.Now().After(deadline) {
					view.TicketStatus = "timeout"
					view.Note = fmt.Sprintf("no ticket after %s; the queue may be slow - re-check with 'alert trace --stuck'", timeout)
					break
				}
				select {
				case <-cmd.Context().Done():
					view.TicketStatus = "timeout"
					view.Note = "interrupted while waiting; re-check with 'alert trace --stuck'"
				case <-time.After(interval):
					continue
				}
				break
			}
			view.Waited = time.Since(start).Round(time.Millisecond).String()

			if ledgerOK {
				rec.TicketID = view.TicketID
				rec.TicketStatus = view.TicketStatus
				rec.CheckedAt = time.Now().UTC()
				if _, err := ledger.UpdateAlert(dir, rec); err != nil {
					fmt.Fprintf(cmd.ErrOrStderr(), "warning: ledger update failed: %v\n", err)
				}
			}
			return printJSONFiltered(cmd.OutOrStdout(), view, flags)
		},
	}
	cmd.Flags().StringVar(&flagAccount, "account", "", "Account id (as provided by the vendor) the alert belongs to")
	cmd.Flags().StringVar(&flagTitle, "title", "", "Alert title (becomes the PSA ticket summary)")
	cmd.Flags().StringVar(&flagDescription, "description", "", "Alert description (becomes the PSA ticket body)")
	cmd.Flags().StringVar(&flagAlertID, "alert-id", "", "Unique alert identifier (auto-generated when omitted)")
	cmd.Flags().Float64Var(&flagPriority, "priority", 2, "Ticket priority: 0-3 (maps to low/medium/high/very high)")
	cmd.Flags().Float64Var(&flagStatus, "status", 0, "Ticket status code: 0-5 (vendor-defined mapping)")
	cmd.Flags().StringVar(&flagDue, "due", "", "Due date-time for the ticket (RFC3339)")
	cmd.Flags().StringVar(&flagService, "service", "", "Relevant serviceId to associate with the alert")
	cmd.Flags().BoolVar(&flagWait, "wait", false, "Poll the ticket-event debug route until the PSA ticket exists or --timeout-wait elapses")
	cmd.Flags().DurationVar(&flagTimeout, "timeout-wait", 2*time.Minute, "Maximum time to wait for the ticket with --wait")
	cmd.Flags().DurationVar(&flagInterval, "interval", 5*time.Second, "Poll interval with --wait")
	return cmd
}
