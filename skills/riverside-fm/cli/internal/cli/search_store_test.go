// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"riverside-fm-pp-cli/internal/cliutil/testenv"
	"riverside-fm-pp-cli/internal/store"
)

// search must read the store the live reads write through to, or it can never
// find anything.
func TestSearchReadsThePressStore(t *testing.T) {
	testenv.Isolate(t)
	s, err := store.Open(defaultDBPath("riverside-fm-cli"))
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := s.UpsertBatch("projects", []json.RawMessage{json.RawMessage(`{"id":"p1","name":"zebrafish episode"}`)}); err != nil {
		t.Fatal(err)
	}
	s.Close()

	cmd := newSearchCmd(&rootFlags{asJSON: true})
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetArgs([]string{"zebrafish"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("search: %v", err)
	}
	if !strings.Contains(out.String(), `"p1"`) {
		t.Fatalf("search did not find the stored project: %s", out.String())
	}
}
