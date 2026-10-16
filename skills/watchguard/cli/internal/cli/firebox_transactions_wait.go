// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// pp:data-source live

package cli

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"watchguard-pp-cli/internal/client"
	"watchguard-pp-cli/internal/cliutil"
)

// Firebox transaction statuses (Firebox Management deployments docs):
// scheduled, pending, in_progress -> complete, or one of the end states that
// are not success: failed, imaged / timed_out (a pending or in-progress
// deployment timed out; the docs renamed imaged to timed_out) and canceled
// (the record was deleted before the deployment executed). Hand-fix
// (handfixes.json transactions-wait-terminal-statuses): imaged and canceled
// used to be polled until --max-wait and reported as still running (exit 9).
var terminalTxStatus = map[string]bool{"complete": true, "failed": true, "imaged": true, "timed_out": true, "canceled": true}

const (
	minWaitInterval = 2 * time.Second
	maxWait         = 4 * time.Hour
)

type waitView struct {
	ID             string   `json:"id"`
	Device         string   `json:"device,omitempty"`
	Object         string   `json:"object,omitempty"`
	Status         string   `json:"status"`
	Settled        bool     `json:"settled"`
	ElapsedSeconds int      `json:"elapsed_seconds"`
	Transitions    []string `json:"transitions"`
}

// newestTransaction picks the transaction with the largest inception_time.
func newestTransaction(rows []map[string]any) map[string]any {
	var best map[string]any
	bestAt := -1
	for _, r := range rows {
		at, _ := intField(r, "inception_time")
		if at > bestAt {
			best, bestAt = r, at
		}
	}
	return best
}

// transactionDeviceID maps FB-12345 to the numeric ID the transactions query
// documents; FireClusters keep their full FBCL-12345 ID.
func transactionDeviceID(device string) string {
	d := strings.TrimSpace(device)
	if strings.HasPrefix(strings.ToUpper(d), "FB-") {
		return d[3:]
	}
	return d
}

func getTransaction(ctx context.Context, c *client.Client, own, id string) (map[string]any, error) {
	data, err := c.GetNoCache(ctx, "/rest/firebox/management/v1/commands/"+url.PathEscape(own)+"/transactions/"+url.PathEscape(id), nil)
	if err != nil {
		return nil, err
	}
	tx := firstObject(data, "data")
	if tx == nil || (strField(tx, "id") == "" && strField(tx, "status") == "") {
		return nil, notFoundErr(fmt.Errorf("transaction %s not found", id))
	}
	return tx, nil
}

func newNovelFireboxTransactionsWaitCmd(flags *rootFlags) *cobra.Command {
	var device, interval, maxWaitFlag string
	cmd := &cobra.Command{
		Use:   "wait [transaction-id]",
		Short: "Block until a Firebox deployment or install transaction completes or fails",
		Long: "Polls a Firebox Management transaction until it reaches an end state: complete, failed, imaged or timed_out (the deployment timed out) or canceled.\n\n" +
			"Pass a transaction ID (e.g. ddply_12345_J2VwPbXuek3CRGN1), or --device to wait on that device's newest deployment transaction.\n" +
			"Exit codes for this command: 0 complete, 8 ended without completing - failed, imaged, timed_out or canceled, reported in status (do not treat it as still running), 9 still running when --max-wait elapsed.",
		Example: strings.Trim(`
  watchguard-cli firebox-transactions wait --device FB-12345 --max-wait 10m
  watchguard-cli firebox-transactions wait ddply_12345_J2VwPbXuek3CRGN1 --interval 5s --json`, "\n"),
		Annotations: map[string]string{
			"mcp:read-only":          "true",
			"pp:data-source":         "live",
			"pp:typed-exit-codes":    "0,8,9",
			"pp:no-error-path-probe": "true",
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 && cmd.Flags().NFlag() == 0 {
				return cmd.Help()
			}
			if dryRunOK(flags) {
				return writeDryRun(cmd.OutOrStdout(), flags, "firebox-transactions wait")
			}
			if len(args) == 0 && device == "" {
				_ = cmd.Usage()
				return usageErr(fmt.Errorf("pass a transaction ID or --device"))
			}
			every, err := cliutil.ParseDurationLoose(interval)
			if err != nil || every <= 0 {
				return usageErr(fmt.Errorf("--interval: %q is not a duration", interval))
			}
			limit, err := cliutil.ParseDurationLoose(maxWaitFlag)
			if err != nil || limit <= 0 {
				return usageErr(fmt.Errorf("--max-wait: %q is not a duration", maxWaitFlag))
			}
			every = max(every, minWaitInterval)
			limit = min(limit, maxWait)
			if cliutil.IsDogfoodEnv() {
				limit = min(limit, 20*time.Second)
			}
			c, err := flags.newClient()
			if err != nil {
				return err
			}
			own, err := ownAccount(c)
			if err != nil {
				return err
			}
			ctx, cancel := context.WithTimeout(cmd.Context(), limit+time.Minute)
			defer cancel()
			id := ""
			if len(args) > 0 {
				id = args[0]
			} else {
				data, err := c.GetNoCache(ctx, "/rest/firebox/management/v1/commands/"+url.PathEscape(own)+"/transactions",
					map[string]string{"device": transactionDeviceID(device), "tx_type": "deployment"})
				if err != nil {
					return classifyAPIErrorOnly(err)
				}
				// Hand-fix (handfixes.json: fanout-malformed-collection-is-failed-read):
				// a malformed row must not hand the wait an older, finished transaction.
				rows, lerr := listRows(data, "data")
				if lerr != nil {
					return apiErr(fmt.Errorf("listing deployment transactions for device %s: %w", device, lerr))
				}
				tx := newestTransaction(rows)
				if tx == nil {
					return notFoundErr(fmt.Errorf("no deployment transactions found for device %s", device))
				}
				id = strField(tx, "id")
			}
			start := time.Now()
			deadline := start.Add(limit)
			view := waitView{ID: id, Transitions: make([]string, 0)}
			for {
				pollCtx, pollCancel := context.WithDeadline(ctx, deadline)
				tx, err := getTransaction(pollCtx, c, own, id)
				pollCancel()
				if err != nil {
					if errors.Is(err, context.DeadlineExceeded) && ctx.Err() == nil {
						break // --max-wait ran out mid-request: report the last status (exit 9)
					}
					return classifyAPIErrorOnly(err)
				}
				status := strField(tx, "status")
				view.Device, view.Object = strField(tx, "device"), strField(tx, "object")
				if status != view.Status {
					view.Status = status
					view.Transitions = append(view.Transitions, status)
					fmt.Fprintf(cmd.ErrOrStderr(), "%s  %s %s\n", time.Now().Format(time.Kitchen), id, status)
				}
				view.ElapsedSeconds = int(time.Since(start).Seconds())
				remaining := time.Until(deadline)
				if terminalTxStatus[status] || remaining <= 0 {
					break
				}
				select {
				case <-ctx.Done():
					return ctx.Err()
				case <-time.After(min(every, remaining)):
				}
				if !time.Now().Before(deadline) {
					break // no request is sent after --max-wait
				}
			}
			view.Settled = terminalTxStatus[view.Status]
			if err := printJSONFiltered(cmd.OutOrStdout(), view, flags); err != nil {
				return err
			}
			switch {
			case view.Settled && view.Status != "complete":
				return &cliError{code: 8, err: fmt.Errorf("transaction %s ended %s", id, view.Status)}
			case !view.Settled:
				return &cliError{code: 9, err: fmt.Errorf("transaction %s still %s after %s", id, view.Status, limit)}
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&device, "device", "", "Wait on the newest deployment transaction for this device: 12345 or FB-12345, FBCL-12345 for a FireCluster")
	cmd.Flags().StringVar(&interval, "interval", "10s", "Time between status checks (minimum 2s)")
	cmd.Flags().StringVar(&maxWaitFlag, "max-wait", "10m", "Give up (exit 9) after this long (maximum 4h)")
	return cmd
}
