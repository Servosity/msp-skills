// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
//
// Hand-written (NOT generated): pinned by skills/saas-alerts/handfixes.json
// under the "human-renderers-scrub-terminal" entry.

package cli

import "io"

// saasScrubWriter drops terminal control bytes from the human output of the
// novel commands (triage, cross-tenant, investigate, coverage, the Respond
// queue, decide, contain, subscriptions health). Their lines interpolate
// tenant strings (user, customer, rule and device names, event fields) and
// none of them emits styling, so every ESC, BEL or other C0 control except
// tab and newline is attacker-reachable text. UTF-8 C1 controls are already
// escaped by c1EscapeWriter below this writer.
type saasScrubWriter struct{ w io.Writer }

func (s saasScrubWriter) Unwrap() io.Writer { return s.w }

func (s saasScrubWriter) Write(p []byte) (int, error) {
	buf := make([]byte, 0, len(p))
	for _, b := range p {
		if (b < 0x20 && b != '\t' && b != '\n') || b == 0x7f {
			continue
		}
		buf = append(buf, b)
	}
	if _, err := s.w.Write(buf); err != nil {
		return 0, err
	}
	return len(p), nil
}

// saasHumanWriter wraps w for a novel command's human renderer.
func saasHumanWriter(w io.Writer) io.Writer { return saasScrubWriter{w: w} }
