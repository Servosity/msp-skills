// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
//
// Hand-written (NOT generated): credkit-wiring receipts.

package platform

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"superops-pp-cli/internal/credkit"
)

// A receipt never keeps a held credential: not in argv (a --deliver webhook's
// userinfo), not in a warning.
func TestReceiptNeverKeepsAHeldCredential(t *testing.T) {
	if !heldChild(t) {
		return
	}
	const user, pass = "r~V", "ReceiptHookPass4"
	credkit.Hold("minted:test", user, pass)
	root := setPlatformRoots(t)
	paths, err := PathsFor("synthetic", "sample-pp-cli", "sample")
	if err != nil {
		t.Fatal(err)
	}
	session := &Session{Profile: &Profile{Name: "synthetic"}, ProfileName: "synthetic", CLI: "sample", Source: "sample", GateOutcome: GateVerified, Paths: paths}
	receiptPath := filepath.Join(root, "receipt.json")
	hook := "webhook:https://" + user + ":" + pass + "@hooks.example.test/x"
	writer, err := NewReceiptWriter(session, ReceiptOptions{CLI: ReceiptCLI{Name: "sample-pp-cli", Version: "test"}, Argv: []string{"sample-pp-cli", "--deliver", hook, "list"}, ReceiptFile: receiptPath})
	if err != nil {
		t.Fatal(err)
	}
	if err := writer.Finalize(context.Background(), "success", 0, nil, OutputMetadata{Warnings: []string{"retry with " + pass}}, RateLimitMetadata{}); !errors.Is(err, credkit.ErrWithheld) {
		t.Fatal("a receipt repeating a held value was written")
	}
	data, _ := os.ReadFile(receiptPath)
	for _, v := range []string{user, pass} {
		if strings.Contains(string(data), v) {
			t.Fatalf("receipt keeps %q:\n%s", v, data)
		}
	}
}
