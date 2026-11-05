// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cliutil

import "testing"

// Each line of the value is exempt on its own (a JSON literal), so only the whole
// value protects it. The file does not parse, so the scan is all there is.
func TestMultiLineValueHeldWhole(t *testing.T) {
	cases := []struct{ name, file, want string }{
		{"closed", "api_key = \"\"\"true\nfalse\nnull\"\"\"\nbroken =\n", "true\nfalse\nnull"},
		{"never closed", "api_key = \"\"\"true\nfalse\nnull\n", "true\nfalse\nnull\n"},
		{"literal", "api_key = \x27\x27\x27true\nfalse\x27\x27\x27\nbroken =\n", "true\nfalse"},
		{"escape", "api_key = \"\"\"a\\tb\nc\"\"\"\nbroken =\n", "a\tb\nc"},
		{"line-ending backslash", "api_key = \"\"\"true\\\n   false\"\"\"\nbroken =\n", "truefalse"},
		{"whitespace after the backslash", "api_key = \"\"\"4829\\   \n1506\"\"\"\nbroken =\n", "48291506"},
		{"whitespace after a later backslash", "api_key = \"\"\"48\\\n  29\\   \n1506\"\"\"\nbroken =\n", "48291506"},
		{"crlf", "api_key = \"\"\"true\r\nfalse\r\nnull\"\"\"\r\nbroken =\r\n", "true\r\nfalse\r\nnull"},
		{"crlf line-ending backslash", "api_key = \"\"\"4829\\ \r\n  1506\"\"\"\r\nbroken =\r\n", "48291506"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			found := false
			for _, v := range scanConfigValues(c.file) {
				found = found || v.source == "config:api_key" && v.value == c.want
			}
			if !found {
				t.Errorf("no value %q in %q: %+v", c.want, c.file, scanConfigValues(c.file))
			}
		})
	}
}
