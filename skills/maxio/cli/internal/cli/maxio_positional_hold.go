// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

// Hand-written (NOT generated; handfixes.json credential-positional-held): a
// positional argument whose usage name is a credential, such as
// `one-time-tokens <chargify_token>`, is held before the command runs, so a
// response or an error that repeats the request path cannot print it.

package cli

import (
	"regexp"
	"strings"

	"github.com/spf13/cobra"

	"maxio-pp-cli/internal/cliutil"
	"maxio-pp-cli/internal/credkit"
)

var usageArgName = regexp.MustCompile(`<([^<>\s]+)>`)

// holdCredentialPositionals holds each such argument. An undeclared flag may or
// may not take the next argument, so both readings are held.
func holdCredentialPositionals(root *cobra.Command, args []string) {
	cmd, _, err := root.Find(args)
	if err != nil || cmd == nil {
		return
	}
	var slots []int
	for i, m := range usageArgName.FindAllStringSubmatch(cmd.Use, -1) {
		if cliutil.SecretFieldName(m[1]) {
			slots = append(slots, i)
		}
	}
	if len(slots) == 0 {
		return
	}
	depth := len(strings.Fields(cmd.CommandPath())) - 1 // the command path words, the root excluded
	for _, consumeUnknown := range []bool{true, false} {
		pos := positionalArgs(cmd, args, consumeUnknown)
		for _, s := range slots {
			if depth+s < len(pos) {
				credkit.Hold("positional:credential", pos[depth+s])
			}
		}
	}
}

func positionalArgs(cmd *cobra.Command, args []string, consumeUnknown bool) []string {
	var pos []string
	for i := 0; i < len(args); i++ {
		a := args[i]
		if a == "--" {
			return append(pos, args[i+1:]...)
		}
		if !strings.HasPrefix(a, "-") || len(a) == 1 {
			pos = append(pos, a)
			continue
		}
		name, _, hasValue := strings.Cut(strings.TrimLeft(a, "-"), "=")
		f := cmd.Flags().Lookup(name)
		if f == nil {
			f = cmd.InheritedFlags().Lookup(name)
		}
		isBool := f != nil && f.Value.Type() == "bool"
		if !hasValue && !isBool && (f != nil || consumeUnknown) {
			i++ // the flag's value
		}
	}
	return pos
}
