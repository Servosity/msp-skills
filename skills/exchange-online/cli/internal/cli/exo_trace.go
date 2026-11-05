// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"context"
	"fmt"
	"time"

	"exchange-online-pp-cli/internal/client"
)

// Get-MessageTraceV2 has no OData paging: a query covers at most 10 days and
// returns at most ResultSize rows, newest first. The next rows come from the
// same query with EndDate and StartingRecipientAddress taken from the last row
// (Microsoft Learn get-messagetracev2). The slice-and-cursor walk mirrors CIPP's
// Get-CIPPBecMessageTrace.ps1, including its stop on a cursor that does not advance.

const (
	tracePageSize = 1000
	traceMaxSlice = 10 * 24 * time.Hour
)

var traceMaxRows = 20000

type traceResult struct {
	Rows     []exoRow
	Complete bool
	Note     string
}

func traceTime(t time.Time) string { return t.UTC().Format("2006-01-02T15:04:05") }

// traceWindow returns every trace row matching filter received in [start, end].
func traceWindow(ctx context.Context, c *client.Client, tenant string, filter exoRow, start, end time.Time) (traceResult, error) {
	res := traceResult{Rows: []exoRow{}, Complete: true}
	seen := map[string]bool{}
	for sliceEnd := end; sliceEnd.After(start); {
		sliceStart := start
		if sliceEnd.Sub(start) > traceMaxSlice {
			sliceStart = sliceEnd.Add(-traceMaxSlice)
		}
		params := exoRow{"StartDate": traceTime(sliceStart), "EndDate": traceTime(sliceEnd), "ResultSize": tracePageSize}
		for k, v := range filter {
			params[k] = v
		}
		prev := ""
		for {
			batch, err := exoGet(ctx, c, exoBeta(tenant, "Get-MessageTraceV2"), params, "")
			if err != nil {
				return res, err
			}
			added := 0
			for _, r := range batch {
				key := rs(r, "MessageTraceId") + "|" + rs(r, "RecipientAddress") + "|" + rs(r, "Received")
				if !seen[key] {
					seen[key] = true
					res.Rows = append(res.Rows, r)
					added++
				}
			}
			if len(batch) < tracePageSize {
				break
			}
			if len(res.Rows) >= traceMaxRows {
				res.Complete, res.Note = false, fmt.Sprintf("stopped after %d trace rows; narrow the window or filter by sender or recipient", traceMaxRows)
				return res, nil
			}
			last := batch[len(batch)-1]
			received := rs(last, "Received")
			_, ok := parseExoTime(received)
			rcpt := rs(last, "RecipientAddress")
			cursor := received + "|" + rcpt
			if added == 0 || !ok || rcpt == "" || cursor == prev {
				res.Complete, res.Note = false, "message trace paging stalled (cursor did not advance); results are partial"
				return res, nil
			}
			prev = cursor
			// Pass Received back verbatim: rounding it would skip rows in the same second.
			params["EndDate"] = received
			params["StartingRecipientAddress"] = rcpt
		}
		sliceEnd = sliceStart
	}
	return res, nil
}
