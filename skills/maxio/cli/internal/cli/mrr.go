package cli

import (
	"github.com/spf13/cobra"
)

// newNovelMrrCmd is the parent of the MRR command group: now, waterfall,
// client, and sync. Hand-authored (replaces the generated stub); the
// constructor name is referenced by root.go's AddCommand wiring.
func newNovelMrrCmd(flags *rootFlags) *cobra.Command {
	cmd := &cobra.Command{
		Use:         "mrr",
		Short:       "Recurring revenue: current MRR, the movement waterfall, per-client revenue, and snapshot sync",
		Annotations: map[string]string{"mcp:read-only": "true"},
		RunE:        parentNoSubcommandRunE(flags),
	}
	cmd.AddCommand(newNovelMrrNowCmd(flags))
	cmd.AddCommand(newNovelMrrWaterfallCmd(flags))
	cmd.AddCommand(newNovelMrrClientCmd(flags))
	cmd.AddCommand(newNovelMrrSyncCmd(flags))
	return cmd
}
