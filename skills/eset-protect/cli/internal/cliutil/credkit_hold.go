// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

// Hand-written (NOT generated; handfixes.json credkit-wiring).
// The connector reads no document of its own: a config, credential or profile
// file and a request body go to credkit.HoldBody, a URL to credkit.HoldURL.
// What stays here is what the kit has no reader for, each with the reason
// (.review/kit-findings.md).

package cliutil

import (
	"encoding/base64"
	"encoding/hex"
	"encoding/pem"
	"fmt"
	"io"
	"os"
	"regexp"
	"slices"
	"strings"

	"eset-protect-pp-cli/internal/credkit"

	"github.com/pelletier/go-toml/v2"
)

// Hold, HoldBody and HoldURL are the kit's. Production code never reassigns them.
// In-process unit tests do, to no-ops: a held value lasts for the process and the
// kit withholds whatever carries it, so one test's fixture would withhold the next
// test's output. The built binaries' leak tests (internal/credwire) prove the real thing.
var (
	Hold     = credkit.Hold
	HoldBody = credkit.HoldBody
	HoldURL  = credkit.HoldURL
)

// HoldConfigFile holds a config, credential or profile file's credentials before
// it is parsed, with HoldBody. Kept beside it: a [headers] value and the
// password or a query value inside base_url, which contract 1 names and HoldBody
// does not read from a config (.review/kit-findings.md, 2026-11-04).
func HoldConfigFile(data []byte) {
	HoldBody(data)
	var parsed struct {
		BaseURL string         `toml:"base_url"`
		Headers map[string]any `toml:"headers"`
	}
	if toml.Unmarshal(data, &parsed) != nil {
		return // bytes that do not parse: HoldBody held every string in them
	}
	HoldURL("userinfo", parsed.BaseURL) // not config:base_url, which an Identifier exempts
	for name, v := range parsed.Headers {
		switch v.(type) {
		case string, int64, float64:
			Hold("header:"+strings.ToLower(name), fmt.Sprint(v))
		}
	}
}

// HoldFile is HoldConfigFile on a file. Only a regular file is opened, so a FIFO
// named on the command line cannot hang startup.
func HoldFile(path string) {
	if data, err := readRegularFile(path); err == nil {
		HoldConfigFile(data)
	}
}

// HoldTLSKeyFile holds the private key the MCP server would serve TLS with: each
// PEM body line and the decoded key's text forms. Kept: the kit reads no PEM, and
// holding the raw DER runs its startup collision check out of budget
// (TestHTTPSModeStarts). Armor and header lines are not the key, and holding them
// would withhold any PEM text. Only a regular file is opened.
func HoldTLSKeyFile(path string) {
	data, err := readRegularFile(path)
	if err != nil {
		return
	}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line != "" && !strings.HasPrefix(line, "-----") && !strings.Contains(line, ":") {
			Hold("minted:tls-key", line)
		}
	}
	for rest := data; ; {
		block, more := pem.Decode(rest)
		if block == nil {
			return
		}
		Hold("minted:tls-key", hex.EncodeToString(block.Bytes), base64.StdEncoding.EncodeToString(block.Bytes), base64.RawURLEncoding.EncodeToString(block.Bytes))
		rest = more
	}
}

func readRegularFile(path string) ([]byte, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("%s is not a regular file", path)
	}
	return os.ReadFile(path) // #nosec G304 -- the operator's own config, store or key path, read to hold its values
}

// ReadRequestBody reads a --stdin request body and holds it with HoldBody
// before the caller can preview or send it. --stdin is not in
// Declarations.StdinFlags: the kit holds each line of a declared stdin, and a
// pretty-printed body's "{" line then refuses every start.
func ReadRequestBody(r io.Reader) ([]byte, error) {
	data, err := io.ReadAll(r)
	HoldBody(data)
	return data, err
}

// HoldSetTokenStdin holds the token auth set-token is about to read from a pipe,
// before cobra can echo it in a flag error, and hands the same bytes back on
// os.Stdin. Kept: Declarations.StdinFlags keys on a flag, and set-token reads
// stdin with none (.review/kit-findings.md, 2026-11-04).
func HoldSetTokenStdin(args []string) {
	if !slices.Contains(args, "set-token") || slices.Contains(args, "--help") || slices.Contains(args, "-h") {
		return
	}
	if fi, err := os.Stdin.Stat(); err != nil || fi.Mode()&os.ModeCharDevice != 0 {
		return
	}
	data, err := io.ReadAll(os.Stdin)
	if err != nil {
		return // set-token reports the same read failure
	}
	for _, v := range append([]string{string(data)}, strings.Split(string(data), "\n")...) {
		if v = strings.TrimSpace(v); v != "" {
			Hold("stdin", v) // a blank line held would withhold all output
		}
	}
	r, w, err := os.Pipe()
	if err != nil {
		return
	}
	go func() {
		_, _ = w.Write(data)
		_ = w.Close()
	}()
	os.Stdin = r
}

// secretProfileKey names a flag whose value is a credential by its name: the run
// profile allow-list refuses to save one (handfixes.json run-profile-allow-list).
var secretProfileKey = regexp.MustCompile(`(?i)passw|secret|token|api.?key|private.?key|access.?key|credential|cookie|authorization|^auth$|auth_?(header|token)`)

// SecretProfileKey reports whether a run profile key names a credential.
func SecretProfileKey(key string) bool { return secretProfileKey.MatchString(key) }
