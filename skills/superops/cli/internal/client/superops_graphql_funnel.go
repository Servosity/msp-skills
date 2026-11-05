// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
//
// Hand-written (NOT generated): GraphQL response errors pass the credential
// mask before any caller can show them. Wired into the generated Query and
// Mutate by reapply/graphql.py (superops-graphql-error-funnel).

package client

import (
	"errors"
)

// funnelGraphQLError is the one place a GraphQL response error becomes
// showable. SuperOps answers a GraphQL error with HTTP 200, so it never takes
// the HTTP error path's mask (shownErrorBody), and its text reaches sync
// events, doctor, raw and MCP results. Every message and code passes
// maskCredentialText; the kit's sinks check the text that leaves. An access denial stays a
// *GraphQLAccessDeniedError so sync and the exit-code classifiers still read
// it as one. Any other error is replaced by its masked text: nothing unwraps
// to the vendor's original wording.
func (c *Client) funnelGraphQLError(err error) error {
	if err == nil {
		return nil
	}
	shown := func(text string) string {
		return c.maskCredentialText(text)
	}
	var denied *GraphQLAccessDeniedError
	if errors.As(err, &denied) {
		out := &GraphQLAccessDeniedError{
			Codes:    make([]string, 0, len(denied.Codes)),
			Messages: make([]string, 0, len(denied.Messages)),
		}
		for _, code := range denied.Codes {
			out.Codes = append(out.Codes, shown(code))
		}
		for _, msg := range denied.Messages {
			out.Messages = append(out.Messages, shown(msg))
		}
		return out
	}
	return maskedError{msg: shown(err.Error())}
}
