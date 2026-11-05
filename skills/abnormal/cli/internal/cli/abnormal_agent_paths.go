// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import "strings"

// agentContextPath is how agent-context prints one of the CLI's own
// directories (handfixes.json agent-context-paths-forward-slash). On Windows a
// backslash path segment such as \001 or \2024 reads as an octal string
// escape, and with a credential held the kit runs out of decode budget on the
// whole description and withholds it. Windows takes forward slashes, so the
// path is printed in that form there. Elsewhere a backslash is an ordinary file
// name character and is kept.
func agentContextPath(goos, p string) string {
	if goos == "windows" {
		return strings.ReplaceAll(p, `\`, "/")
	}
	return p
}
