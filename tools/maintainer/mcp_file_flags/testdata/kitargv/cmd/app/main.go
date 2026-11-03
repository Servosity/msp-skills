// Command app reads files named on its command line four ways. Only the
// first is tied to a flag (the reviewed kit's FlagValues, with a constant
// name, over the process argv itself); the others are os.Args reaching a
// file, whoever wrote them.
package main

import (
	"os"

	"github.com/spf13/cobra"

	"kitargv/internal/credkit"
)

func main() {
	credkit.Start(credkit.Declarations{Files: files})
	for _, p := range credkit.FlagValues(os.Args[1:], "--config") {
		_, _ = os.ReadFile(p) // TIED
	}
	for _, p := range credkit.FlagValues(os.Args[1:], name()) {
		_, _ = os.ReadFile(p) // COMPUTED
	}
	root := &cobra.Command{Use: "app", RunE: func(*cobra.Command, []string) error { built(); return nil }}
	root.Flags().StringVar(&notes, "notes", "", "notes file")
	_ = root.Execute()
	scan(os.Args[1:])
}

var notes string

// built hands FlagValues an argv the program made itself: what it returns
// is --notes, not what the --config flag holds.
func built() {
	for _, p := range credkit.FlagValues([]string{"--config", notes}, "--config") {
		_, _ = os.ReadFile(p) // BUILT
	}
}

// files is the program's Files: its --config, through the kit.
func files(args []string) []string { return credkit.FlagValues(args, "--config") }

func name() string { return os.Getenv("APP_FLAG") }

// scan is a connector's own argv scan: os.Args reaching a file.
func scan(args []string) {
	for i, a := range args {
		if a == "--notes" && i+1 < len(args) {
			_, _ = os.Open(args[i+1]) // OWN
		}
	}
}
