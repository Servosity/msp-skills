// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-written: JAMF_PRO_FLEET_SECRETS, the one variable that carries every
// registered instance's client secret (handfixes.json fleet-secrets-one-declared-variable).

package fleet

import (
	"fmt"
	"strings"

	"jamf-pro-pp-cli/internal/cliutil"
)

// EntriesEnv names the one variable that carries every registered instance's
// client secret. It is read only by its literal name (rawSecrets), so the
// env-schema gate and a reader of the source both see which variable holds
// the credentials, and every install channel can declare it; secrets never
// touch disk.
const EntriesEnv = "JAMF_PRO_FLEET_SECRETS"

// rawSecrets is the one read of JAMF_PRO_FLEET_SECRETS. An MCPB placeholder
// left unresolved reads as unset.
func rawSecrets() string {
	return cliutil.EnvOverride("JAMF_PRO_FLEET_SECRETS")
}

// RawSecrets is the variable's value as this process reads it.
func RawSecrets() string { return rawSecrets() }

// Secrets returns every instance's client secret from JAMF_PRO_FLEET_SECRETS,
// by instance name.
func Secrets() (map[string]string, error) {
	return ParseSecrets(rawSecrets())
}

// SecretFor is the client secret JAMF_PRO_FLEET_SECRETS carries for one
// instance ("" when it has no entry).
func SecretFor(name string) (string, error) {
	m, err := Secrets()
	if err != nil {
		return "", err
	}
	return m[name], nil
}

// SecretsFor is the JAMF_PRO_FLEET_SECRETS value that carries one instance's
// secret and no other: what a process pinned to that instance may see.
func SecretsFor(name, secret string) string {
	return name + "=" + secret
}

// ParseSecrets parses a JAMF_PRO_FLEET_SECRETS value into instance name ->
// client secret.
//
// The value is a list of entries separated by commas and/or newlines; empty
// entries are ignored, and whitespace around names, secrets and separators is
// trimmed. Each entry is name=secret, split at its FIRST "=", so a secret may
// itself contain "=". name must be a valid instance name (the pattern
// 'instance add' uses); secret must be non-empty. An instance may appear once.
// A secret that contains a comma or a newline cannot be expressed.
//
// Errors name the entry's position (counting non-empty entries from 1) and the
// instance name when it is valid. They never repeat secret text, nor a name
// that failed validation (it may be a secret pasted in the wrong place).
func ParseSecrets(raw string) (map[string]string, error) {
	out := map[string]string{}
	at := map[string]int{}
	n := 0
	for _, entry := range splitSecretEntries(raw) {
		n++
		name, secret, ok := strings.Cut(entry, "=")
		if !ok {
			return nil, fmt.Errorf(`%s entry %d has no "=": write each instance as name=secret (a secret cannot contain a comma or a newline)`, EntriesEnv, n)
		}
		name, secret = strings.TrimSpace(name), strings.TrimSpace(secret)
		switch {
		case name == "":
			return nil, fmt.Errorf(`%s entry %d has no instance name before "="`, EntriesEnv, n)
		case !slugPattern.MatchString(name) || strings.Contains(name, ".."):
			return nil, fmt.Errorf(`%s entry %d: the text before "=" is not an instance name (lowercase letters, digits, '.', '_' or '-', max 63)`, EntriesEnv, n)
		case secret == "":
			return nil, fmt.Errorf("%s entry %d (instance %q) has an empty secret", EntriesEnv, n, name)
		}
		if first, dup := at[name]; dup {
			return nil, fmt.Errorf("%s entry %d (instance %q) repeats entry %d; give each instance one secret", EntriesEnv, n, name, first)
		}
		at[name] = n
		out[name] = secret
	}
	return out, nil
}

// splitSecretEntries splits raw at commas and newlines and returns the
// non-empty, trimmed entries.
func splitSecretEntries(raw string) []string {
	var out []string
	for _, e := range strings.FieldsFunc(raw, func(r rune) bool { return r == ',' || r == '\n' }) {
		if e = strings.TrimSpace(e); e != "" {
			out = append(out, e)
		}
	}
	return out
}

// SecretFragments is what to hold when raw does not parse: every piece of raw
// split on commas, newlines and "=" (trimmed, non-empty), and each of those
// split again at whitespace, so a secret survives whatever shape the mistake
// left it in.
func SecretFragments(raw string) []string {
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

// HasSecret reports whether JAMF_PRO_FLEET_SECRETS carries a secret for name
// (false when the variable does not parse).
func HasSecret(name string) bool {
	s, err := SecretFor(name)
	return err == nil && s != ""
}
