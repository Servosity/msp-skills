// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"bytes"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"aws-billing-pp-cli/internal/cliutil/testenv"
)

// sync-layer-outcomes: a sync or workflow archive in which every attempted
// AWS layer failed exits non-zero instead of reporting an empty account, and
// the JSON summary names each layer's outcome. Every AWS call here is refused
// by the endpoint pin (AWS_ENDPOINT_URL points off AWS), so no layer can sync.
func TestSyncAndArchiveFailClosedWhenEveryLayerFailed(t *testing.T) {
	testenv.Isolate(t)
	t.Setenv("AWS_ENDPOINT_URL", "http://127.0.0.1:9")
	t.Setenv("AWS_ACCESS_KEY_ID", "AKIA"+"UNITSYNCOUTCOME1")
	t.Setenv("AWS_SECRET_ACCESS_KEY", "syncOutcomeSecretUnit-3c9e1a")
	t.Setenv("AWS_REGION", "us-east-1")
	t.Setenv("AWS_CONFIG_FILE", "/nonexistent")
	t.Setenv("AWS_SHARED_CREDENTIALS_FILE", "/nonexistent")
	t.Setenv("AWS_MAX_ATTEMPTS", "1")
	for _, args := range [][]string{
		{"sync", "--json", "--skip-inventory"},
		{"sync", "--skip-inventory"},
		{"workflow", "archive", "--json"},
	} {
		cmd := RootCmd()
		var out, errOut bytes.Buffer
		cmd.SetOut(&out)
		cmd.SetErr(&errOut)
		cmd.SetArgs(args)
		err := cmd.Execute()
		if err == nil {
			t.Fatalf("%v: every layer failed and the command exited 0:\n%s", args, out.String())
		}
		if !strings.Contains(err.Error(), "0 of") {
			t.Fatalf("%v: want the all-failed error, got %v", args, err)
		}
		if args[0] == "sync" && args[1] == "--json" {
			var sum syncSummary
			if jerr := json.Unmarshal(out.Bytes(), &sum); jerr != nil {
				t.Fatalf("sync --json printed no summary: %v\n%s", jerr, out.String())
			}
			if len(sum.Layers) != 3 || sum.succeeded() != 0 {
				t.Fatalf("layers = %+v, want accounts, costs and forecasts, all failed", sum.Layers)
			}
		}
	}
}

func TestSyncSummaryOutcomes(t *testing.T) {
	var s syncSummary
	if s.allFailed() != nil {
		t.Fatal("no layer attempted is not a failure")
	}
	s.record("accounts", errors.New("AccessDenied"))
	s.record("costs", nil, errors.New("storing row: disk full"))
	if err := s.allFailed(); err == nil {
		t.Fatal("a store failure must fail the layer")
	}
	s.record("inventory", nil, nil)
	if s.allFailed() != nil || s.succeeded() != 1 {
		t.Fatalf("partial success must stay exit 0: %+v", s.Layers)
	}
	if s.Layers[1].OK || !strings.Contains(s.Layers[1].Error, "disk full") {
		t.Fatalf("costs layer = %+v, want the store error", s.Layers[1])
	}
}
