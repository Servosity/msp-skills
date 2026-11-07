// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
//
// Hand-authored. Saved-policy input for `policies decode --file`.
package cli

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type policyInputZeros struct{}

func (policyInputZeros) Read(p []byte) (int, error) {
	for i := range p {
		p[i] = ' '
	}
	return len(p), nil
}

func TestReadPolicyInputRefusesOversizedInput(t *testing.T) {
	_, err := readPolicyInput(policyInputZeros{})
	if err == nil || !strings.Contains(err.Error(), "larger than") {
		t.Fatalf("oversized input must be refused, got %v", err)
	}
	raw, err := readPolicyInput(io.LimitReader(policyInputZeros{}, policyInputMaxBytes))
	if err != nil || len(raw) != policyInputMaxBytes {
		t.Fatalf("input at the limit must be read whole: len=%d err=%v", len(raw), err)
	}
}

func TestReadPolicyFileReadsSavedPolicy(t *testing.T) {
	path := filepath.Join(t.TempDir(), "policy.json")
	want := `{"policy":{"uuid":"p-1","features":[]}}`
	if err := os.WriteFile(path, []byte(want), 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := readPolicyFile(path)
	if err != nil || string(got) != want {
		t.Fatalf("readPolicyFile = %q, %v", got, err)
	}
	if _, err := readPolicyFile(filepath.Join(t.TempDir(), "absent.json")); err == nil {
		t.Fatal("a missing file must be an error")
	}
}
