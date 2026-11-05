// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-written: PRTG_FLEET_KEYS, the one variable that carries every core's API key.

package fleet

import (
	"fmt"
	"strings"

	"prtg-pp-cli/internal/cliutil"
)

// KeysEnv names the one variable that carries every registered core's API
// key (prtg handfix: fleet-keys-one-declared-variable). It is read only by
// its literal name (rawKeys), so the env-schema gate and a reader of the
// source both see which variable holds the credentials; keys never touch disk.
const KeysEnv = "PRTG_FLEET_KEYS"

// rawKeys is the one read of PRTG_FLEET_KEYS. An MCPB placeholder left
// unresolved reads as unset.
func rawKeys() string {
	return cliutil.EnvOverride("PRTG_FLEET_KEYS")
}

// Keys returns every core's API key from PRTG_FLEET_KEYS, by core name.
func Keys() (map[string]string, error) {
	return ParseKeys(rawKeys())
}

// KeysFor is the PRTG_FLEET_KEYS value that carries one core's key and no
// other: what a process pinned to that core may see.
func KeysFor(name, key string) string {
	return name + "=" + key
}

// ParseKeys parses a PRTG_FLEET_KEYS value into core name -> API key.
//
// The value is a list of entries separated by commas and/or newlines; empty
// entries are ignored, and whitespace around names, keys and separators is
// trimmed. Each entry is name=key, split at its FIRST "=", so a key may itself
// contain "=". name must be a valid core name (the pattern 'core add' uses);
// key must be non-empty. A core may appear once.
//
// A key that contains a comma or a newline cannot be expressed: the
// separator always splits it, and there is no quoting or escape. The piece
// after the split must then stand as its own name=key entry, so in the usual
// case (no "=" after the comma) the whole value is rejected; it is never
// re-joined into one key. Only a key containing ",<core name>=" would read
// as two entries.
//
// Errors name the entry's position (counting non-empty entries from 1) and
// the core name when it is valid. They never repeat key text, nor a name that
// failed validation (it may be a key pasted in the wrong place).
func ParseKeys(raw string) (map[string]string, error) {
	keys := map[string]string{}
	at := map[string]int{}
	n := 0
	for _, entry := range splitKeyEntries(raw) {
		n++
		name, key, ok := strings.Cut(entry, "=")
		if !ok {
			return nil, fmt.Errorf(`%s entry %d has no "=": write each core as name=key (a key cannot contain a comma or a newline)`, KeysEnv, n)
		}
		name, key = strings.TrimSpace(name), strings.TrimSpace(key)
		switch {
		case name == "":
			return nil, fmt.Errorf(`%s entry %d has no core name before "="`, KeysEnv, n)
		case !validName(name):
			return nil, fmt.Errorf(`%s entry %d: the text before "=" is not a core name (lowercase letters, digits, '.', '_' or '-', max 63)`, KeysEnv, n)
		case key == "":
			return nil, fmt.Errorf("%s entry %d (core %q) has an empty key", KeysEnv, n, name)
		}
		if first, dup := at[name]; dup {
			return nil, fmt.Errorf("%s entry %d (core %q) repeats entry %d; give each core one key", KeysEnv, n, name, first)
		}
		at[name] = n
		keys[name] = key
	}
	return keys, nil
}

// splitKeyEntries splits raw at commas and newlines and returns the
// non-empty, trimmed entries.
func splitKeyEntries(raw string) []string {
	var out []string
	for _, e := range strings.FieldsFunc(raw, func(r rune) bool { return r == ',' || r == '\n' }) {
		if e = strings.TrimSpace(e); e != "" {
			out = append(out, e)
		}
	}
	return out
}

// keyFragments is what HoldCredentials holds when raw does not parse: every
// piece of raw split on commas, newlines and "=" (trimmed, non-empty), and
// each of those split again at whitespace, so a key survives whatever shape
// the mistake left it in. Names are held too: nothing in an unparseable value
// can be told apart from a key.
func keyFragments(raw string) []string {
	var out []string
	for _, f := range strings.FieldsFunc(raw, func(r rune) bool { return r == ',' || r == '\n' || r == '=' }) {
		if f = strings.TrimSpace(f); f == "" {
			continue
		}
		out = append(out, f)
		if words := strings.Fields(f); len(words) > 1 {
			out = append(out, words...)
		}
	}
	return out
}
