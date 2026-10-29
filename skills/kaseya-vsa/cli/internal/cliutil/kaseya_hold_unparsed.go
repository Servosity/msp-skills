// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cliutil

import (
	"regexp"
	"strings"
)

// holdUnparsedMinLen keeps short structural words (true, 443, ports, table
// names) out of the held set; an API token secret is far longer.
const holdUnparsedMinLen = 8

var holdUnparsedQuoted = regexp.MustCompile(`"([^"\n]*)"|'([^'\n]*)'`)

// HoldUnparsedValues registers every value-like token of a config or
// credentials file that failed to parse (kaseya-vsa handfix: output-chokepoint,
// startup ordering). A file that cannot be decoded still holds its secrets, so
// the whole file is read, with no size cutoff: every quoted string, every
// right-hand side of a key = value line, and every bare line without "=".
// Over-holding only withholds more output; under-holding leaks.
func HoldUnparsedValues(data []byte) {
	var held []string
	add := func(v string) {
		v = strings.Trim(strings.TrimSpace(v), "\"'[]{}, \t")
		if len(v) >= holdUnparsedMinLen {
			held = append(held, v)
		}
	}
	for _, m := range holdUnparsedQuoted.FindAllStringSubmatch(string(data), -1) {
		add(m[1])
		add(m[2])
	}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(strings.TrimSuffix(line, "\r"))
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if i := strings.Index(line, "="); i >= 0 {
			add(line[i+1:])
			continue
		}
		add(line)
	}
	FunnelHold(held...)
}
