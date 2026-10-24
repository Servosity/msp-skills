// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
//
// Hand-written (NOT generated), cove only: tests for the CLI half of the
// short-credential backstop (cove_short_funnel_cli.go). Hand-fix code:
// cove-short-credential-funnel.

package cli

import (
	"bytes"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"cove-pp-cli/internal/cliutil"
)

// A command's error that repeats a short held credential (one the JSON-RPC
// client itself does not know) is withheld by the backstops Execute wires, in
// the returned error and in everything Cobra prints, and the original error
// stays reachable for exit-code classification.
func TestCoveShortFunnelWiredOverACommandError(t *testing.T) {
	const held = "Zq9!y"
	cliutil.FunnelHold(held)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"id":"jsonrpc","jsonrpc":"2.0","visa":"test-visa-0123456789","error":{"code":-32603,"data":99,"message":"upstream said `+held+`"}}`)
	}))
	t.Cleanup(srv.Close)
	fleetTestEnv(t, srv)
	t.Setenv("COVE_PASSWORD", "secret123456")

	root := newRootCmd(&rootFlags{})
	funnelCommandErrors(root)
	coveShortFunnelCommandErrors(root)
	var printed bytes.Buffer
	root.SetOut(&printed)
	root.SetErr(&printed)
	root.SetArgs([]string{"call", "GetServerInfo", "--no-auth", "--json"})
	err := root.Execute()
	if err == nil || strings.Contains(err.Error(), held) || strings.Contains(printed.String(), held) {
		t.Fatalf("short held credential shown: err=%v printed=%q", err, printed.String())
	}
	if !cliutil.FunnelWithheld(err.Error()) {
		t.Fatalf("the error does not carry the withheld notice: %v", err)
	}
	var cliErr *cliError
	if !errors.As(err, &cliErr) {
		t.Fatalf("the original error is no longer reachable: %#v", err)
	}
}

// The JSON error envelope and an endpoint usage error carry the short
// backstop too; clean text passes unchanged.
func TestCoveShortFunnelOverEnvelopes(t *testing.T) {
	const held = "Wv3&k"
	cliutil.FunnelHold(held)
	var envelope strings.Builder
	writeAPIErrorEnvelope(&envelope, &rootFlags{asJSON: true}, errors.New("server said "+held), 1)
	if strings.Contains(envelope.String(), held) {
		t.Fatalf("short held credential in the API error envelope: %s", envelope.String())
	}
	usage := endpointArgsUsageErr(nil, nil, errors.New("bad arg "+held))
	if strings.Contains(usage.Error(), held) {
		t.Fatalf("short held credential in a usage error: %v", usage)
	}
	envelope.Reset()
	writeAPIErrorEnvelope(&envelope, &rootFlags{asJSON: true}, errors.New("plain failure"), 1)
	if !strings.Contains(envelope.String(), "plain failure") {
		t.Fatalf("clean envelope changed: %s", envelope.String())
	}
	if err := coveShortFunnelError(errors.New("plain failure")); err.Error() != "plain failure" {
		t.Fatalf("clean error changed: %v", err)
	}
}
