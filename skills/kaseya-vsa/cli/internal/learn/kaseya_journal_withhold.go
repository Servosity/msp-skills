// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package learn

// kaseya-vsa handfix (output-chokepoint, review r1 P1-C): every journal
// entry is serialized through the chokepoint, so a held credential typed as
// an argv flag or carried in any field never reaches the journal file. It is
// a method on the entry type, not a line in AppendJournalEntry, so the
// generated declarations stay template-identical. If withholding ever left
// invalid JSON, json.Marshal fails and the entry is not written (fail closed).

import (
	"encoding/json"

	"kaseya-vsa-pp-cli/internal/cliutil"
)

// MarshalJSON is JournalEntry's JSON with every held credential withheld.
func (e JournalEntry) MarshalJSON() ([]byte, error) {
	type plain JournalEntry
	b, err := json.Marshal(plain(e))
	if err != nil {
		return nil, err
	}
	return cliutil.WithholdBytes(b), nil
}
