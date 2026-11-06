// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package client

import (
	"io"
	"net/http"
	"net/url"
	"os"
	"slices"
	"testing"

	"runzero-pp-cli/internal/cliutil"
	"runzero-pp-cli/internal/credkit"
	"runzero-pp-cli/internal/credwire"
)

// TestMain runs this package's unit tests without the credential kit's holds
// and outbound guard (handfixes.json credkit-wiring): no test here runs
// credkit.Start, so a value one test holds would withhold another test's
// writes, and the guard would refuse every credentialed request to the
// loopback servers these tests stand up. The built binaries' black-box tests
// (internal/runzeroblackbox) prove both.
func TestMain(m *testing.M) {
	OutboundGuard = func(rt http.RoundTripper) http.RoundTripper { return rt }
	cliutil.Hold = func(credkit.Source, ...string) {}
	cliutil.HoldBody = func([]byte) {}
	cliutil.HoldURL = func(credkit.Source, string) {}
	os.Exit(m.Run())
}

// TestMaskedFieldsAreTheDeclaredOnes: the response mask names exactly the
// fields the kit holds by name, so a field added to one list reaches the other.
func TestMaskedFieldsAreTheDeclaredOnes(t *testing.T) {
	want := slices.Concat(credwire.Declarations.ResponseFields, credwire.Declarations.CredentialFields)
	if !slices.Equal(maskedFields, want) {
		t.Fatalf("maskedFields = %q, want the declared ResponseFields then CredentialFields %q", maskedFields, want)
	}
}

// TestKitRefusalIsNotRetried: a request the credential kit refused (any
// error that is credkit.ErrWithheld, as net/http wraps it) is a terminal
// refusal, so the retry loop does not replay it.
func TestKitRefusalIsNotRetried(t *testing.T) {
	if !terminalTransportRefusal(&url.Error{Op: "Get", URL: "https://console.runzero.com/api/v1.0/org/assets", Err: credkit.ErrWithheld}) {
		t.Fatal("a kit refusal is retried")
	}
	if terminalTransportRefusal(&url.Error{Op: "Get", URL: "https://console.runzero.com/api/v1.0/org/assets", Err: io.ErrUnexpectedEOF}) {
		t.Fatal("a transient failure is not retried")
	}
}
