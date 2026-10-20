// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
//
// Hand-authored (not generator output); pinned by skills/eset-protect/handfixes.json
// under "human-printers-flatten-tenant-lines".
//
// The terminal writer (c1_escape.go) neutralizes ESC, BEL, CR, C1 and bidi
// controls but lets LF and tab through, because the CLI's own output needs
// them. A tenant string (a device, group or customer name, an incident title)
// that carries LF or tab could therefore add a line or shift a column in a
// hand-written human printer: a device named "X\nPREVIEW (nothing sent):
// StartNetworkIsolation on HARMLESS-KIOSK" forged a second line in the respond
// isolate preview a human approves before --yes. The generic printer already
// flattens cells (helpers.go plainCellValue); the hand-written printers go
// through esetFprintf / esetFprintln, which flatten every string, error and
// Stringer ARGUMENT onto one line while the format string keeps the CLI's own
// newlines and tabs. JSON output never goes through here: encoding/json
// escapes LF and tab itself.
package cli

import (
	"fmt"
	"io"
	"reflect"
	"strings"
)

// esetLineBreaks are the characters a terminal or a reader treats as a line
// or column break inside one value: LF, CR, tab, VT, FF, NEL and the Unicode
// line and paragraph separators.
var esetLineBreaks = strings.NewReplacer(
	"\r\n", " ", "\n", " ", "\r", " ", "\t", " ", "\v", " ", "\f", " ",
	"\u0085", " ", "\u2028", " ", "\u2029", " ",
)

// esetFlatten puts one tenant-controlled value on one line.
func esetFlatten(s string) string {
	if !strings.ContainsAny(s, "\r\n\t\v\f\u0085\u2028\u2029") {
		return s
	}
	return esetLineBreaks.Replace(s)
}

// esetHumanArgs flattens the string-like arguments of a human printer call.
func esetHumanArgs(args []any) []any {
	out := make([]any, len(args))
	for i, a := range args {
		switch v := a.(type) {
		case string:
			out[i] = esetFlatten(v)
		case []string:
			flat := make([]string, len(v))
			for j, s := range v {
				flat[j] = esetFlatten(s)
			}
			out[i] = flat
		case error:
			out[i] = esetFlatten(v.Error())
		case fmt.Stringer:
			out[i] = esetFlatten(v.String())
		default:
			// A named string type prints as its value under %s and %v.
			if rv := reflect.ValueOf(a); rv.IsValid() && rv.Kind() == reflect.String {
				out[i] = esetFlatten(rv.String())
				continue
			}
			out[i] = a
		}
	}
	return out
}

// esetFprintf is fmt.Fprintf for hand-written human output: the format's own
// newlines and tabs stay, every argument is flattened onto one line.
func esetFprintf(w io.Writer, format string, args ...any) (int, error) {
	return fmt.Fprintf(w, format, esetHumanArgs(args)...)
}

// esetFprintln is fmt.Fprintln with every argument flattened onto one line.
func esetFprintln(w io.Writer, args ...any) (int, error) {
	return fmt.Fprintln(w, esetHumanArgs(args)...)
}
