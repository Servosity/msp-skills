// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
//
// Hand-written (NOT generated): pinned by skills/dnsfilter/handfixes.json
// under the "mcp-failure-keeps-stdout-remedy" and
// "mcp-read-only-hinted-tools-filter-credentials" entries.

package cobratree

import (
	"strings"

	"dnsfilter-pp-cli/internal/mcp/bound"
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
// unchanged, unless forced: the call serves a tool advertised
// readOnlyHint=true (redact.Forced), which gets the same rules in every mode
// (handfixes.json: mcp-read-only-hinted-tools-filter-credentials).
func redactChildOutput(s string, forced bool) string {
	raw, final := redact.Raw, redact.Final
	if !redact.Enabled() {
		if !forced {
			return s
		}
		raw, final = redact.RawAlways, redact.FinalAlways
	}
	if s == "" {
		return s
	}
	lines := strings.Split(raw(s), "\n")
	for i, line := range lines {
		if strings.TrimSpace(line) == "" {
			continue
		}
		if block := final(line); redact.IsWithheldBlock(block) {
			lines[i] = block
		}
	}
	out := strings.Join(lines, "\n")
	if block := final(out); redact.IsWithheldBlock(block) {
		return block
	}
	return out
}

// boundTextFor returns bound.Text, or bound.TextAlways for a call of a tool
// advertised readOnlyHint=true (redact.Forced): its captured output (stdout
// as the result, stderr in an error) is redacted before it is budgeted or
// previewed in every mode, as the read-only switch does.
func boundTextFor(forced bool) func(string) string {
	if forced {
		return bound.TextAlways
	}
	return bound.Text
}
