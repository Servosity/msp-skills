// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package fleet

// threecx handfix (every-input-reported): the one rule every fleet verdict
// (health checks, licence renewal window, version drift, backup flags)
// follows: a verdict is healthy only when every input it uses was reported.
// A missing input (a null or absent vendor field: every 3CX field is
// nullable) makes the verdict unknown, or keeps a worse one; it never reads
// as pass, warn or current, and it never drops a row from a filtered report.

import "strings"

// Settle applies the rule to one verdict: missing names the inputs that were
// not reported. With none missing it returns level and detail unchanged;
// otherwise level is raised to Unknown (a worse level stays) and the detail
// names what was not reported.
func Settle(level Level, detail string, missing ...string) (Level, string) {
	if len(missing) == 0 {
		return level, detail
	}
	if Worse(Unknown, level) {
		level = Unknown
	}
	note := "not reported: " + strings.Join(missing, ", ")
	if detail == "" {
		return level, note
	}
	return level, detail + "; " + note
}

// unreported returns the names whose reported flag is false, in order.
func unreported(pairs ...any) []string {
	var out []string
	for i := 0; i+1 < len(pairs); i += 2 {
		if ok, _ := pairs[i+1].(bool); !ok {
			out = append(out, pairs[i].(string))
		}
	}
	return out
}
