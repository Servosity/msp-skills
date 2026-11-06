// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

// Package credkit is the shared credential kit. Every connector vendors it
// byte-identical at cli/internal/credkit (tools/maintainer/vendor_credkit.py)
// and wires it in one hand-written file. It enforces the credential leak
// contract, docs/credential-leak-contract.md; each part names the section it
// enforces:
//
//   - held.go, config.go: the held set (section 1).
//   - match.go, decode.go: the matcher (sections 2 and 3).
//   - limits.go: every limit on a check and every reading bound (section 3, Budget).
//   - public.go: the public-text table and its collision rules (section 2).
//   - stdio*.go, files.go, sql.go, mcp.go: the output layer (sections 4 and 5).
//   - transport.go: the outbound guard (section 2(c)).
//   - exec.go: child processes (sections 1 and 5).
//   - start.go: the startup prologue (section 1 timing, section 4).
//   - declare.go: the per-connector declarations (section 7).
//
// Every entry point is a plain package-level function called with plain
// arguments, so a go/types checker finds each declaration and each routed
// sink by its call site: no reflection, no registration through method
// values.
package credkit

import (
	"errors"
	"os"
)

// ExitInstallFailed is the exit status when the output layer cannot be
// installed: the program exits before writing anything (contract 4).
const ExitInstallFailed = 70

// Sink names. A notice names its sink and nothing about the value.
const (
	SinkStdout = "stdout"
	SinkStderr = "stderr"
	SinkMCP    = "mcp"
	SinkSQLite = "sqlite"
	SinkFile   = "file"
	SinkHTTP   = "outbound request"
	SinkChild  = "child process"
)

// Notice is the fixed text that replaces a withheld unit (contract 4).
func Notice(sink string) string {
	return "[credkit: withheld from " + sink + ": it carried a credential this program holds]"
}

// ErrWithheld reports a unit the kit withheld. Every error the kit returns
// for a withheld unit is ErrWithheld under errors.Is, and its text is the
// sink's notice: it never carries the value.
var ErrWithheld = errors.New(Notice("this sink"))

type withheldError struct{ sink string }

func (e withheldError) Error() string        { return Notice(e.sink) }
func (e withheldError) Is(target error) bool { return target == ErrWithheld }

// fatal ends the process before anything is written (contract 4: if the
// output layer cannot be installed, the program exits before writing
// anything). It is a variable so a test can observe it.
var fatal = func() { os.Exit(ExitInstallFailed) }
