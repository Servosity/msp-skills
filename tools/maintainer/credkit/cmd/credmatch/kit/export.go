// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

//go:build credmatch

// This file is not built here. class_secret_corpus.py copies it into a
// temporary copy of the kit package (beside the kit's own non-test files),
// so the credmatch command reaches the kit's matcher without the kit
// exporting it to connectors. It adds no matching of its own (contract 8).

package credkit

// MatchUnit is the kit's verdict on one unit for a held set: "clean",
// "leak" with the contract clause, or "exhausted" (contract 3, Budget). A
// request is an outbound request judged by contract 2(c) alone, every value
// in held being one the request's origin may not carry.
func MatchUnit(held []string, unit []byte, request bool) (verdict, clause string) {
	var m *matcher
	if request {
		m = newRequestMatcher(held)
	} else {
		m = newMatcher(held)
	}
	switch m.check(unit) {
	case leak:
		if request {
			return "leak", "2(c)"
		}
		// 2(a) when a value is there whole (a short one as a whole token),
		// else a piece of 8 or more: 2(b).
		switch newWholeMatcher(held).check(unit) {
		case leak:
			return "leak", "2(a)"
		case exhausted:
			return "leak", "2(a) or 2(b)"
		}
		return "leak", "2(b)"
	case exhausted:
		return "exhausted", "3 (budget)"
	}
	return "clean", ""
}

// PublicAsWritten is the kit's public-text rule for a value acquired while
// running (contract 2, Public text): true when the value, as written, occurs
// in one public-text entry, so it is recorded, not held.
func PublicAsWritten(v string) bool { return inPublicText(v) }
