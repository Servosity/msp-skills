// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cliutil

// dropsuite handfix (mcp-write-confirm; Codex review round 1 class
// read-call-reaches-a-mint-route): the client marks the context of every call
// it issues as a mutation (doInternal with mutationIntent: GetMutating, the
// generated sign-in link commands and endpoint tools, locate --sso-link,
// evidence --with-sso-link). The API transport (internal/cli/ds_guard.go)
// lets only a marked request reach a route that changes Dropsuite state.

import (
	"context"
	"errors"
)

// ErrRequestRefused marks a request this program refused to send (the API
// origin pin, the route guard). A replay would be refused the same way, so
// the client returns it at once instead of retrying (client.go
// terminalTransportRefusal).
var ErrRequestRefused = errors.New("request refused")

type mutationKey struct{}

// MarkMutation returns ctx marked as carrying a mutation when mutation is
// true, and ctx unchanged otherwise.
func MarkMutation(ctx context.Context, mutation bool) context.Context {
	if !mutation || ctx == nil {
		return ctx
	}
	return context.WithValue(ctx, mutationKey{}, true)
}

// MutationIntent reports whether ctx was marked by MarkMutation.
func MutationIntent(ctx context.Context) bool {
	if ctx == nil {
		return false
	}
	v, _ := ctx.Value(mutationKey{}).(bool)
	return v
}
