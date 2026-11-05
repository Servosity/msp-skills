// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-authored (not generated): pax8 handfix (journal-credential-hardening).

package learn

// The learn journal keeps no text the operator typed in place of a name the
// program declares. The generated journal.go calls into this file from one
// line per site:
//
//   - JournalSessionKey stores the LEARN_SESSION override as "o:" plus the
//     same bounded hash harness session ids get under "h:" (hashHarnessSession),
//     so a session still groups its entries but the value never reaches disk;
//   - JournalArgvShape keeps a flag name only when the invoked command declares
//     it (journalFlagDeclared), records any other as journalUndeclaredFlag, and
//     stops at "--";
//   - JournalInvocation never keeps a failed flag as typed
//     (journalOpaqueFailedFlag);
//
// Operator-named text that repeats a credential the program holds (the
// journal, teach.log, playbooks, the learn tables) is withheld by the shared
// credential kit at the sink (handfixes.json credkit-wiring): AppendJournalEntry
// appends one complete entry through credkit.AppendFile.

// journalUndeclaredFlag is the argv_shape key for every flag name the invoked
// command is not shown to declare: a name typed in a flag's place may be a
// credential, so only this placeholder is kept.
const journalUndeclaredFlag = "<undeclared>"

// journalFlagDeclared resolves which flag names argv_shape may keep: the
// caller's declared-flag lookup when it passes one. Otherwise only the names
// isBoolFlag answers true for: the Cobra caller's isBoolFlag is a lookup in
// the invoked command's own flag registry (Lookup, then InheritedFlags, then
// ShorthandLookup for one letter; the tree sets no name normalization), so a
// true answer is a declaration of exactly that name. A declared flag that
// takes a value is then recorded as the placeholder until the caller passes
// its declared-flag lookup. With neither lookup no name is kept.
func journalFlagDeclared(isBoolFlag func(name string) bool, isDeclared []func(name string) bool) func(name string) bool {
	for _, declared := range isDeclared {
		if declared != nil {
			return declared
		}
	}
	if isBoolFlag != nil {
		return isBoolFlag
	}
	return func(string) bool { return false }
}

// journalOpaqueFailedFlag is the failed_flag a journal entry keeps. A failed
// flag is a name the invoked command does not declare, so it is never kept as
// typed. When Cobra matched it to a declared flag (suggested is set), the
// placeholder plus the hash of the typed name stays: the learn loop still
// pairs the correction, and two typos stay two candidates. Any other failed
// flag is dropped.
func journalOpaqueFailedFlag(failed, suggested string) string {
	if failed == "" || suggested == "" {
		return ""
	}
	return "--" + journalUndeclaredFlag + ":" + hashHarnessSession(failed)
}
