// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"errors"
	"net"
	"net/url"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

// An error the program did not construct (net/http's url.Error keeps a short
// username) prints without the URL userinfo once the command tree is wrapped,
// and still classifies through errors.As.
func TestCommandErrorsNeverShowURLUserinfo(t *testing.T) {
	inner := &net.OpError{Op: "dial", Net: "tcp", Err: errors.New("connect: refused")}
	raw := &url.Error{Op: "Post", URL: "http://svc:pw@127.0.0.1:9/msp", Err: inner}
	root := &cobra.Command{Use: "superops-cli"}
	leaf := &cobra.Command{Use: "leaf", RunE: func(*cobra.Command, []string) error { return raw }}
	root.AddCommand(leaf)
	redactCommandErrorURLUserinfo(root)
	err := leaf.RunE(leaf, nil)
	if err == nil {
		t.Fatal("the wrapped command lost its error")
	}
	if strings.Contains(err.Error(), "svc") || strings.Contains(err.Error(), "pw@") {
		t.Fatalf("command error shows URL userinfo: %s", err)
	}
	if !strings.Contains(err.Error(), "127.0.0.1:9") {
		t.Fatalf("command error lost the host: %s", err)
	}
	var opErr *net.OpError
	if !errors.As(err, &opErr) {
		t.Fatalf("command error no longer unwraps to the transport error: %#v", err)
	}
}
