// Package credkit stands in for the vendored credential kit: only the shape
// of FlagValues, Declarations.Files and Start matters to the analyzer, which
// reads a call to FlagValues with a constant flag as that flag's getter, and
// Start's d.Files call as the functions the module stores in Files, when told
// the kit is the reviewed one.
package credkit

import (
	"os"
	"strings"
)

// Declarations names the program's credential files.
type Declarations struct {
	Files func(args []string) []string
}

// Start reads every file d.Files names.
func Start(d Declarations) {
	keep(&d)
	if d.Files != nil {
		for _, f := range d.Files(os.Args[1:]) {
			_, _ = os.ReadFile(f) // KITREAD
		}
	}
}

func keep(*Declarations) {}

// FlagValues is every value args give flag.
func FlagValues(args []string, flag string) []string {
	var out []string
	for i, a := range args {
		if a == flag && i+1 < len(args) {
			out = append(out, args[i+1])
		} else if v, ok := strings.CutPrefix(a, flag+"="); ok {
			out = append(out, v)
		}
	}
	return out
}
