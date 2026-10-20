// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
//
// Hand-authored. Attaches `detections spread` under the generated
// `detections` resource. The generated parent may or may not wire the novel
// command itself; addNovelCommandIfAbsent makes this hook a no-op when it
// already did, and keeps the command reachable when a regeneration drops it.
package cli

import "github.com/spf13/cobra"

func init() {
	registerNovelCommand(func(root *cobra.Command, flags *rootFlags) {
		if detections, _, err := root.Find([]string{"detections"}); err == nil && detections != nil && detections != root {
			addNovelCommandIfAbsent(detections, newNovelDetectionsSpreadCmd(flags))
		}
	})
}
