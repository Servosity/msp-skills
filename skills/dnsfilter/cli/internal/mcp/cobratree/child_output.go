// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
//
// Hand-written (NOT generated): pinned by skills/dnsfilter/handfixes.json
// under the "mcp-failure-keeps-stdout-remedy" entry.

package cobratree

import (
	"strings"

	"dnsfilter-pp-cli/internal/mcp/redact"
)

// redactChildOutput redacts a companion CLI's captured stdout or stderr as a
// WHOLE, under the read-only switch, before any caller selects lines, takes a
// tail or cuts bytes from it:
//
//   - redact.Raw on the full text replaces every credential-named field of
//     every JSON value in it (an NDJSON line, a pretty-printed object spread
//     over several lines);
//   - a line that still names a credential with a value (a record cut short
//     by the capture cap, key=value text) is replaced by the withheld
//     message, so the other lines stay usable;
//   - if the text as a whole still trips the funnel's rule (a CSV header
//     naming a credential column, a pair split across lines), all of it is
//     withheld, because a later tail or cut could separate a value from the
//     header or key that marks it.
//
// The capture cap itself only drops a suffix, which never separates a value
// from the key or header in front of it. Off the switch the text is returned
// unchanged.
func redactChildOutput(s string) string {
	if s == "" || !redact.Enabled() {
		return s
	}
	lines := strings.Split(redact.Raw(s), "\n")
	for i, line := range lines {
		if strings.TrimSpace(line) != "" && redact.Final(line) == redact.WithheldBlockMessage {
			lines[i] = redact.WithheldBlockMessage
		}
	}
	out := strings.Join(lines, "\n")
	if redact.Final(out) == redact.WithheldBlockMessage {
		return redact.WithheldBlockMessage
	}
	return out
}
