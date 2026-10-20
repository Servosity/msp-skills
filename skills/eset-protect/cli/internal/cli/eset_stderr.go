// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"io"
	"os"
)

// esetStderr receives the generated commands' error and warning lines
// (failed import records, API errors, partial-sync warnings), which can echo
// tenant text back from ESET. Execute points it at the terminal-escaping,
// secret-masking stderr writer (handfixes.json: stderr-errors-escaped);
// carriage-return progress lines still write os.Stderr directly so they are
// not held back by line buffering.
var esetStderr io.Writer = os.Stderr

// esetDefaultPageSize is the page size ESET Connect uses when pageSize is
// unspecified or 0 (handfixes.json: local-read-page-size-is-not-a-total).
const esetDefaultPageSize = 50
