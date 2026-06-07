// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-implemented novel feature for skykick-cli (stub replaced; survives regen-merge).
package cli

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"skykick-pp-cli/internal/cliutil"
	"skykick-pp-cli/internal/fleet"
)

type watchOperationView struct {
	OperationID string          `json:"operation_id"`
	Status      string          `json:"status"`
	Terminal    bool            `json:"terminal"`
	Succeeded   bool            `json:"succeeded"`
	Polls       int             `json:"polls"`
	ElapsedSecs float64         `json:"elapsed_seconds"`
	LastBody    json.RawMessage `json:"last_response,omitempty"`
	Note        string          `json:"note,omitempty"`
}

func newNovelWatchOperationCmd(flags *rootFlags) *cobra.Command {
	var timeoutSecs, intervalSecs int

	cmd := &cobra.Command{
		Use:   "watch-operation <operationId>",
		Short: "Poll an async operation to a terminal state with backoff in one command",
		Long: strings.Trim(`
Use this to block until an async operation (returned by 'backup
discover-mailboxes' or 'backup discover-sites') reaches a terminal state.
Polls GET /operation/{id} with exponential backoff (interval grows 1.5x up to
30s). It does not start discovery - run the discover command first to get the
operationId. Exits non-zero when the operation fails or the timeout elapses.
`, "\n"),
		Example: strings.Trim(`
  skykick-cli watch-operation 1a2b3c4d-0000-0000-0000-000000000000 --timeout 300
  skykick-cli watch-operation 1a2b3c4d-0000-0000-0000-000000000000 --agent
`, "\n"),
		Annotations: map[string]string{
			"mcp:read-only": "true",
			"pp:happy-args": "<operationId>=1a2b3c4d-0000-0000-0000-000000000000",
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 && cmd.Flags().NFlag() == 0 {
				return cmd.Help()
			}
			if len(args) < 1 {
				_ = cmd.Usage()
				return usageErr(fmt.Errorf("operationId is required"))
			}
			if dryRunOK(flags) {
				fmt.Fprintf(cmd.OutOrStdout(), "would poll /operation/%s until terminal (timeout %ds)\n", args[0], timeoutSecs)
				return nil
			}
			opID := strings.TrimSpace(args[0])
			if timeoutSecs <= 0 {
				_ = cmd.Usage()
				return usageErr(fmt.Errorf("--timeout must be a positive number of seconds"))
			}
			if intervalSecs <= 0 {
				intervalSecs = 5
			}

			c, err := flags.newClient()
			if err != nil {
				return err
			}

			// Live-dogfood curtail: one poll, no wait loop.
			maxElapsed := time.Duration(timeoutSecs) * time.Second
			if cliutil.IsDogfoodEnv() {
				maxElapsed = 0
			}

			start := time.Now()
			interval := time.Duration(intervalSecs) * time.Second
			view := watchOperationView{OperationID: opID}
			for {
				data, err := c.Get(cmdContext(cmd), "/operation/"+opID, nil)
				if err != nil {
					return classifyAPIError(err, flags)
				}
				view.Polls++
				view.LastBody = data
				status, _ := fleet.ExtractOperationStatus(data)
				view.Status = status
				view.Terminal, view.Succeeded = fleet.OperationTerminal(status)
				view.ElapsedSecs = time.Since(start).Seconds()

				if view.Terminal {
					break
				}
				if status == "" {
					view.Note = "response carried no recognizable status field; treating as non-terminal"
				}
				if time.Since(start)+interval > maxElapsed {
					view.Note = fmt.Sprintf("timeout: operation not terminal after %d polls in %.0fs", view.Polls, view.ElapsedSecs)
					if err := fleetPrint(cmd, flags, view, nil, view.Note); err != nil {
						return err
					}
					return apiErr(fmt.Errorf("operation %s not terminal within %ds (last status %q)", opID, timeoutSecs, status))
				}
				select {
				case <-cmdContext(cmd).Done():
					return cmdContext(cmd).Err()
				case <-time.After(interval):
				}
				interval = time.Duration(float64(interval) * 1.5)
				if interval > 30*time.Second {
					interval = 30 * time.Second
				}
			}

			summary := fmt.Sprintf("operation %s: %s after %d polls (%.0fs)", opID, view.Status, view.Polls, view.ElapsedSecs)
			if err := fleetPrint(cmd, flags, view, nil, summary); err != nil {
				return err
			}
			if !view.Succeeded {
				return apiErr(fmt.Errorf("operation %s ended in non-success state %q", opID, view.Status))
			}
			return nil
		},
	}
	cmd.Flags().IntVar(&timeoutSecs, "timeout", 300, "Maximum seconds to wait for a terminal state")
	cmd.Flags().IntVar(&intervalSecs, "interval", 5, "Initial poll interval in seconds (backs off 1.5x to max 30s)")
	return cmd
}
