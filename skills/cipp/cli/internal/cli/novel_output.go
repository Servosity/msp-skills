// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"encoding/json"
	"sort"

	"github.com/spf13/cobra"
)

// printNovelJSON prints a novel command's result through the generated
// output pipeline, keeping every field the command chose to emit under
// --agent/--compact. The generic high-gravity allow-list would otherwise
// strip a curated report down to its ids (scorecard lost result and
// synced_at, changes lost changed_fields, offboard-check its leftovers).
func printNovelJSON(cmd *cobra.Command, flags *rootFlags, v any) error {
	raw, err := json.Marshal(v)
	if err != nil {
		return err
	}
	var decoded any
	if err := json.Unmarshal(raw, &decoded); err != nil {
		return err
	}
	keys := map[string]bool{}
	collectKeys(decoded, 0, keys)
	keep := make([]string, 0, len(keys))
	for k := range keys {
		keep = append(keep, k)
	}
	sort.Strings(keep)
	return printJSONFilteredKeep(cmd.OutOrStdout(), v, flags, keep...)
}

// collectKeys gathers object keys down to two object levels (a report row
// and the objects nested in it).
func collectKeys(v any, depth int, keys map[string]bool) {
	switch x := v.(type) {
	case map[string]any:
		if depth >= 2 {
			return
		}
		for k, child := range x {
			keys[k] = true
			collectKeys(child, depth+1, keys)
		}
	case []any:
		for _, child := range x {
			collectKeys(child, depth, keys)
		}
	}
}
