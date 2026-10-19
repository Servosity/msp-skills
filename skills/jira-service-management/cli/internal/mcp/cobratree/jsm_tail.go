// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cobratree

import "strings"

// jsmTailSinglePoll makes an MCP tail call poll once and return. tail follows
// by default and cliArgsFromMCP forwards only true booleans, so follow:false
// was dropped and the tool call never returned while the child CLI kept
// polling the API. Any --follow token is removed and --follow=false is placed
// right after the command path.
func jsmTailSinglePoll(argv []string, prefixLen int) []string {
	out := make([]string, 0, len(argv)+1)
	out = append(out, argv[:prefixLen]...)
	out = append(out, "--follow=false")
	terminated := false
	for _, a := range argv[prefixLen:] {
		if a == "--" {
			terminated = true
		}
		if !terminated && (a == "--follow" || strings.HasPrefix(a, "--follow=")) {
			continue
		}
		out = append(out, a)
	}
	return out
}
