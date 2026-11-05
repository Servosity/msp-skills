// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cliutil

// prtg handfix (bulk-action-outcomes): a per-call record of failed write
// results. The MCP server opens one for each tool call; the API transport
// reports every write response with a failed item into the call's record,
// whichever handler sent it, and the server then fails the call.

import (
	"context"
	"sync"
)

// ActionFailures collects the failure summaries of one call's write requests.
type ActionFailures struct {
	mu        sync.Mutex
	summaries []string
}

type actionFailuresKey struct{}

// WithActionFailures returns ctx carrying a fresh record.
func WithActionFailures(ctx context.Context) (context.Context, *ActionFailures) {
	a := &ActionFailures{}
	return context.WithValue(ctx, actionFailuresKey{}, a), a
}

// ReportActionFailure records summary on ctx's record, if it carries one.
func ReportActionFailure(ctx context.Context, summary string) {
	if a, ok := ctx.Value(actionFailuresKey{}).(*ActionFailures); ok && a != nil {
		a.mu.Lock()
		a.summaries = append(a.summaries, summary)
		a.mu.Unlock()
	}
}

// Summaries returns what was recorded.
func (a *ActionFailures) Summaries() []string {
	a.mu.Lock()
	defer a.mu.Unlock()
	return append([]string(nil), a.summaries...)
}
