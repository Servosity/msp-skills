// Command app's Files is a variable: the analyzer cannot name the function
// the kit calls, so the call stays dynamic and argv reaches the kit's read.
package main

import "kitfilesvar/internal/credkit"

var pick = files

func main() { credkit.Start(credkit.Declarations{Files: pick}) }

func files(args []string) []string { return credkit.FlagValues(args, "--config") }
