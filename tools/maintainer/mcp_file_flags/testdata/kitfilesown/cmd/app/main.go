// Command app's Files scans argv itself: os.Args reaching the kit's read.
package main

import "kitfilesown/internal/credkit"

func main() { credkit.Start(credkit.Declarations{Files: files}) }

func files(args []string) []string {
	var out []string
	for i, a := range args {
		if a == "--config" && i+1 < len(args) {
			out = append(out, args[i+1])
		}
	}
	return out
}
