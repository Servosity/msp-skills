// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cobratree

import (
	"reflect"
	"testing"
)

// zoom-phone hand-fix: zoom-phone-mcp-write-confirm. The CLI's --confirm in
// an MCP child accepts only the bare flag, which is what this server emits for
// the JSON boolean true; every other type arrives as --confirm=<value> and is
// left a preview. If this encoding changes, confirmed calls fail closed
// (preview only) and this test names why.
func TestZoomConfirmArgvEncoding(t *testing.T) {
	cases := map[string]struct {
		in   any
		want []string
	}{
		"bool true":   {true, []string{"--confirm"}},
		"bool false":  {false, nil},
		"string 1":    {"1", []string{"--confirm=1"}},
		"string true": {"true", []string{"--confirm=true"}},
		"number 1":    {float64(1), []string{"--confirm=1"}},
		"array true":  {[]any{true}, []string{"--confirm=true"}},
	}
	for name, c := range cases {
		got := cliArgsFromMCP(map[string]any{"confirm": c.in}, nil)
		if !reflect.DeepEqual(got, c.want) {
			t.Errorf("%s: argv %q; want %q", name, got, c.want)
		}
	}
}
