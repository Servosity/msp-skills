// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
//
// Hand-written (NOT generated): pinned by skills/dnsfilter/handfixes.json under
// the "base-url-credentials-held-and-redacted" entry (class "secret-echo").

package cli

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

var errBaseURLSentinel = errors.New("sentinel cause")

// TestCommandErrorsRedactURLCredentials: a command error that names a URL
// with credentials is printed with them redacted, before the kit judges it
// (so a held password yields the redacted error, not a withheld one), and the
// original error stays reachable for exit-code classification.
func TestCommandErrorsRedactURLCredentials(t *testing.T) {
	const pw = "CliWriterPw4242"
	root := &cobra.Command{Use: "root", SilenceUsage: true}
	root.AddCommand(&cobra.Command{
		Use: "fail",
		RunE: func(*cobra.Command, []string) error {
			return errors.Join(errBaseURLSentinel, errors.New(`creating request: parse "ht!tp://u:`+pw+`@[::1/v1/policies/all": first path segment in URL cannot contain colon`))
		},
	})
	redactCommandErrors(root)
	var stderr bytes.Buffer
	root.SetErr(&stderr)
	root.SetOut(&bytes.Buffer{})
	root.SetArgs([]string{"fail"})
	err := root.Execute()
	if err == nil {
		t.Fatal("the failing command returned no error")
	}
	for _, shown := range []string{err.Error(), stderr.String()} {
		if strings.Contains(shown, pw) {
			t.Fatalf("command error shows the URL password: %s", shown)
		}
		if !strings.Contains(shown, `parse "ht!tp://****@[::1/v1/policies/all"`) {
			t.Fatalf("command error lost its redacted URL (withheld whole instead?): %s", shown)
		}
	}
	if !errors.Is(err, errBaseURLSentinel) {
		t.Fatal("the redacted error no longer unwraps to its cause")
	}
}

// TestAPIErrorEnvelopeRedactsURLCredentials: the --json error envelope a
// command prints itself shows a URL's credentials redacted.
func TestAPIErrorEnvelopeRedactsURLCredentials(t *testing.T) {
	var buf bytes.Buffer
	writeAPIErrorEnvelope(&buf, &rootFlags{asJSON: true}, errors.New(`GET /v1/x: Get "http://127.0.0.1:9/?api_key=EnvelopeKey4242": connection refused`), 5)
	if strings.Contains(buf.String(), "EnvelopeKey4242") {
		t.Fatalf("error envelope shows the URL credential: %s", buf.String())
	}
	if !strings.Contains(buf.String(), "api_key=****") {
		t.Fatalf("error envelope lost its redacted URL: %s", buf.String())
	}
}

// TestUsageErrorEnvelopeRedactsURLCredentials: the --json usage envelope an
// argument error prints shows a URL's credentials redacted.
func TestUsageErrorEnvelopeRedactsURLCredentials(t *testing.T) {
	var buf bytes.Buffer
	cmd := &cobra.Command{Use: "x"}
	cmd.Flags().Bool("json", false, "")
	cmd.SetOut(&buf)
	err := endpointArgsUsageErr(cmd, &rootFlags{asJSON: true}, errors.New(`bad argument "https://u:UsagePw4242@h.example/x"`))
	for _, shown := range []string{buf.String(), err.Error()} {
		if strings.Contains(shown, "UsagePw4242") {
			t.Fatalf("usage error shows the URL password: %s", shown)
		}
	}
	if !strings.Contains(buf.String(), "https://****@h.example/x") {
		t.Fatalf("usage envelope lost its redacted URL: %s", buf.String())
	}
}
