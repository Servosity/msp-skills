// Command app writes into its own argv before the kit's FlagValues scans it:
// what FlagValues returns for --config is then --notes, so the scan is not
// tied to --config, and --notes reaching the read is reported.
package main

import (
	"os"

	"github.com/spf13/cobra"

	"kitargvmut/internal/credkit"
)

var notes string

func main() {
	root := &cobra.Command{Use: "app", RunE: func(*cobra.Command, []string) error { mutated(); return nil }}
	root.Flags().StringVar(&notes, "notes", "", "notes file")
	_ = root.Execute()
}

func mutated() {
	argv := os.Args[1:]
	argv[0] = "--config=" + notes
	for _, p := range credkit.FlagValues(argv, "--config") {
		_, _ = os.ReadFile(p) // MUTATED
	}
}
