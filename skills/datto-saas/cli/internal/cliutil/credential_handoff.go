// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cliutil

// Hand-written (handfixes.json credkit-wiring): the credential kit starts no
// child with a held value in its environment, and a child gets the held set but
// not the name each value goes by. So the MCP server takes the API keys out of a
// CLI child's environment and writes them to the child's stdin as JSON, and the
// child sets them again before anything reads it.

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"strings"

	"datto-saas-pp-cli/internal/credkit"
)

var handoffEnvNames = []string{"DATTO_SAAS_PUBLIC_KEY", "DATTO_SAAS_SECRET_KEY"}

func handedOff(name string) bool {
	for _, n := range handoffEnvNames {
		if name == n {
			return true
		}
	}
	return false
}

// CredentialHandoff takes the API keys out of env and returns what is left with
// the JSON for the child's stdin. The MCP HTTP bearer is dropped and never
// handed: it is this server's alone. A base URL with a password stays held in
// the environment, so the kit refuses that child: it fails closed.
func CredentialHandoff(env []string) ([]string, io.Reader) {
	out := make([]string, 0, len(env))
	handed := map[string]string{}
	for _, entry := range env {
		name, value, _ := strings.Cut(entry, "=")
		switch {
		case strings.EqualFold(name, "DATTO_SAAS_MCP_HTTP_TOKEN"):
		case handedOff(name):
			if value != "" {
				handed[name] = value
			}
		default:
			out = append(out, entry)
		}
	}
	data, _ := json.Marshal(handed) // a map of strings always marshals
	return out, bytes.NewReader(data)
}

// ReadCredentialHandoff takes what the MCP server wrote to stdin and sets it as
// this process's environment, held. It runs inside credkit.Start, before
// anything can print. Only an MCP child reads it, and a terminal has nothing to
// read, so a marker set by hand cannot hang the CLI. It reads to the end, so a
// command that reads stdin sees an empty stream, as it did with no input. Any
// name but the API keys is ignored: stdin can never set a base URL or a path.
func ReadCredentialHandoff() {
	if !IsMCPChildEnv() {
		return
	}
	if fi, err := os.Stdin.Stat(); err != nil || fi.Mode()&os.ModeCharDevice != 0 {
		return
	}
	data, err := io.ReadAll(os.Stdin)
	if err != nil {
		return
	}
	// Every value of every JSON value to the end of input, each duplicate included,
	// is held before a map keeps the last one. Anything but exactly one object of
	// strings is also held whole and word by word, and sets nothing.
	var found bodyValues
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()
	ok := walkBody(dec, true, 0, &found)
	for ok && dec.More() {
		ok = walkBody(dec, true, 0, &found)
	}
	for _, v := range found.secrets {
		Hold("stdin", v)
	}
	var handed map[string]string
	if !ok || json.Unmarshal(data, &handed) != nil {
		credkit.HoldBody(data)
		return
	}
	for _, name := range handoffEnvNames {
		if v, ok := handed[name]; ok {
			Hold(credkit.Source("env:"+name), v)
			_ = os.Setenv(name, v)
		}
	}
}
