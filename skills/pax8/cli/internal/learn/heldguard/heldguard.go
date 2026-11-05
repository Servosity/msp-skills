// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-authored (not generated): pax8 handfix (journal-credential-hardening).

// Package heldguard is the one refusal every learn-loop writer shares:
// operator-supplied text (a session label, a flag name, a query family, an
// alias, a pattern, a teach-log field) that repeats a credential this process
// holds is refused before it is stored, never stored rewritten. The match is
// cliutil.HeldEchoes, the output chokepoint's own test (every form the output
// filter decodes, whole or split across neighbouring text); this package adds
// no matcher of its own. It lives in its own package so internal/learn and its
// lookups and patterns subpackages call the same function without an import
// cycle.
package heldguard

import (
	"errors"
	"strings"

	"pax8-pp-cli/internal/cliutil"
)

// ErrHeld is returned in place of storing text that repeats a held value. Its
// wording avoids the words the generated error classifiers read as an auth
// failure.
var ErrHeld = errors.New("not saved: it repeats a secret value this program holds")

// Held reports whether any of texts, or all of them read together (a value
// split across two fields), repeats a value the process holds.
func Held(texts ...string) bool {
	for _, t := range texts {
		if cliutil.HeldEchoes(t) {
			return true
		}
	}
	return len(texts) > 1 && cliutil.HeldEchoes(strings.Join(texts, "\n"))
}

// Refuse returns ErrHeld when Held(texts...) and nil otherwise.
func Refuse(texts ...string) error {
	if Held(texts...) {
		return ErrHeld
	}
	return nil
}
