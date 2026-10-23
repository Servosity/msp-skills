// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
//
// Hand-written (NOT generated): pinned by skills/aws-billing/handfixes.json
// under the "secret-echo-decoded-funnel" entry (class "secret-echo").

package awsx

import (
	"context"
	"os"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/smithy-go/middleware"

	"aws-billing-pp-cli/internal/cliutil"
)

// funnelSDKErrors adds one middleware pair to every AWS operation: the
// finalize step records the credentials the request is signed with, and the
// initialize step (the outermost) passes any operation error through the
// shared secret-echo funnel. AWS answers can quote the caller's access key id
// (IncompleteSignature, SignatureDoesNotMatch) and a proxy can echo anything
// it was sent; an error whose text carries a held credential, in any encoding
// the funnel decodes, is withheld before any caller can print it.
func funnelSDKErrors(creds aws.CredentialsProvider) func(*middleware.Stack) error {
	return func(stack *middleware.Stack) error {
		if err := stack.Initialize.Add(middleware.InitializeMiddlewareFunc("SecretEchoFunnel",
			func(ctx context.Context, in middleware.InitializeInput, next middleware.InitializeHandler) (middleware.InitializeOutput, middleware.Metadata, error) {
				held := &heldCredentials{}
				out, md, err := next.HandleInitialize(context.WithValue(ctx, heldCredentialsKey{}, held), in)
				if err != nil {
					if w := cliutil.FunnelWithhold(err.Error(), held.values()); cliutil.FunnelWithheld(w) {
						err = withheldSDKError{err: err, msg: w}
					}
				}
				return out, md, err
			}), middleware.Before); err != nil {
			return err
		}
		return stack.Finalize.Add(middleware.FinalizeMiddlewareFunc("SecretEchoFunnelCredentials",
			func(ctx context.Context, in middleware.FinalizeInput, next middleware.FinalizeHandler) (middleware.FinalizeOutput, middleware.Metadata, error) {
				if held, ok := ctx.Value(heldCredentialsKey{}).(*heldCredentials); ok && creds != nil {
					// The signer retrieves the same (cached) credentials next.
					if v, err := creds.Retrieve(ctx); err == nil {
						held.signed = []string{v.AccessKeyID, v.SecretAccessKey, v.SessionToken}
					}
				}
				return next.HandleFinalize(ctx, in)
			}), middleware.Before)
	}
}

type heldCredentialsKey struct{}

// heldCredentials is what one operation could have sent: the credentials it
// was signed with, plus the static credentials in the environment.
type heldCredentials struct {
	signed []string
}

func (h *heldCredentials) values() []string {
	out := append([]string{}, h.signed...)
	for _, name := range []string{"AWS_ACCESS_KEY_ID", "AWS_SECRET_ACCESS_KEY", "AWS_SESSION_TOKEN"} {
		out = append(out, os.Getenv(name))
	}
	return out
}

// withheldSDKError replaces an error's text while keeping the original
// reachable through errors.As, so access-denied and throttle classification
// (IsAccessDenied, ErrorCode) still works.
type withheldSDKError struct {
	err error
	msg string // a cliutil.FunnelWithhold notice
}

func (e withheldSDKError) Error() string { return e.msg }
func (e withheldSDKError) Unwrap() error { return e.err }
