// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

// Hand-authored tests for the concise `search` projection (issue #101 / #84
// residual item #2): the default view is id/name/type/match, and --full
// restores whole raw records.

package cli

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"axcient-pp-cli/internal/store"
	"github.com/spf13/cobra"
)

func TestProjectSearchHit_DeviceShape(t *testing.T) {
	hit := store.SearchHit{
		ResourceType: "device",
		Data:         json.RawMessage(`{"id_":7654321,"name":"Acme-FS01","status":"NORMAL"}`),
	}
	row := projectSearchHit(hit, "Acme")
	if row.ID != "7654321" {
		t.Fatalf("ID = %q, want 7654321 (numeric id_ must render as an integer, not scientific notation)", row.ID)
	}
	if row.Name != "Acme-FS01" {
		t.Fatalf("Name = %q, want Acme-FS01", row.Name)
	}
	if row.Type != "device" {
		t.Fatalf("Type = %q, want device", row.Type)
	}
	if row.Match != "name~Acme" {
		t.Fatalf("Match = %q, want name~Acme", row.Match)
	}
}

func TestMatchIndicator(t *testing.T) {
	obj := map[string]any{"name": "Acme-FS01", "note": "hello world"}
	if got := matchIndicator(obj, "Acme"); got != "name~Acme" {
		t.Fatalf("matchIndicator(Acme) = %q, want name~Acme", got)
	}
	if got := matchIndicator(obj, "world"); got != "note~world" {
		t.Fatalf("matchIndicator(world) = %q, want note~world", got)
	}
	if got := matchIndicator(obj, "zzz"); got != "" {
		t.Fatalf("matchIndicator(no-substring) = %q, want empty", got)
	}
}

func newSearchRenderCmd() (*cobra.Command, *bytes.Buffer) {
	cmd := &cobra.Command{}
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(new(bytes.Buffer))
	return cmd, &out
}

// A bytes.Buffer stdout is non-TTY, so renderSearchResults takes the jsonMode
// path deterministically — exactly the agent/piped surface the projection
// protects from raw-record token burn.
func TestRenderSearchResults_DefaultProjectsAwayRawFields(t *testing.T) {
	hits := []store.SearchHit{{
		ResourceType: "device",
		Data:         json.RawMessage(`{"id_":7654321,"name":"Acme-FS01","verbose_blob":"WALL_OF_NESTED_JSON"}`),
	}}
	cmd, out := newSearchRenderCmd()

	if err := renderSearchResults(cmd, &rootFlags{}, hits, 50, DataProvenance{Source: "local"}, false, "Acme"); err != nil {
		t.Fatalf("renderSearchResults: %v", err)
	}
	got := out.String()
	for _, want := range []string{"Acme-FS01", "device", "7654321", "name~Acme"} {
		if !strings.Contains(got, want) {
			t.Fatalf("projection missing %q in:\n%s", want, got)
		}
	}
	if strings.Contains(got, "verbose_blob") || strings.Contains(got, "WALL_OF_NESTED_JSON") {
		t.Fatalf("projection leaked raw record fields (should be id/name/type/match only):\n%s", got)
	}
}

func TestRenderSearchResults_FullRestoresRawRecords(t *testing.T) {
	hits := []store.SearchHit{{
		ResourceType: "device",
		Data:         json.RawMessage(`{"id_":7654321,"name":"Acme-FS01","verbose_blob":"WALL_OF_NESTED_JSON"}`),
	}}
	cmd, out := newSearchRenderCmd()

	if err := renderSearchResults(cmd, &rootFlags{}, hits, 50, DataProvenance{Source: "local"}, true, "Acme"); err != nil {
		t.Fatalf("renderSearchResults --full: %v", err)
	}
	if got := out.String(); !strings.Contains(got, "verbose_blob") {
		t.Fatalf("--full should return whole records, got:\n%s", got)
	}
}
