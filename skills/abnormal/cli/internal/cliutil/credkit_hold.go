// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

// Hand-written (NOT generated; handfixes.json credkit-wiring). The connector's
// few calls into the shared credential kit that are not a sink: a config or
// credential file is held with the kit's own readers (credkit.HoldBody and
// credkit.HoldURL) every time it is read, and startup's reading of --config
// and --home is kept so a run cannot use another file than the one the kit
// took the allowed origin from.

package cliutil

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"abnormal-pp-cli/internal/credkit"

	"github.com/pelletier/go-toml/v2"
)

// Hold is credkit.Hold, and holdBody and holdURL are credkit.HoldBody and
// credkit.HoldURL. Production code never reassigns them. In-process unit tests
// call NoHolds: a held value lasts for the process and the kit withholds
// whatever carries it, so one test's fixture would withhold the next test's
// output. The built binaries' leak tests (internal/credwire) prove the real thing.
var (
	Hold     = credkit.Hold
	holdBody = credkit.HoldBody
	holdURL  = credkit.HoldURL
)

// NoHolds makes every hold in this package a no-op. For in-process unit tests only.
func NoHolds() {
	Hold = func(credkit.Source, ...string) {}
	holdBody = func([]byte) {}
	holdURL = func(credkit.Source, string) {}
}

// HoldConfigFile holds what a config or credential file carries, before it is
// parsed: the declared credential keys (credkit.HoldBody; a file that does not
// parse has every value held), the password and query values of base_url, and
// every [headers] value, which the client sends on each request.
func HoldConfigFile(data []byte) {
	holdBody(data)
	var f struct {
		BaseURL string            `toml:"base_url"`
		Headers map[string]string `toml:"headers"`
	}
	if toml.Unmarshal(data, &f) != nil {
		return // holdBody held every value of a file that does not parse
	}
	if f.BaseURL != "" {
		// Under "userinfo", the kit's own source for a base URL's password:
		// config:base_url is a declared Identifier, and the kit holds nothing
		// under an Identifier. HoldURL also reads a password the kit's start
		// check misses, such as one with a raw "/" in it.
		holdURL("userinfo", f.BaseURL)
	}
	for k, v := range f.Headers {
		Hold("header:"+strings.ToLower(k), v)
	}
}

// HoldConfigFilePath is HoldConfigFile on a file this run may not use. Only a
// regular file is opened, so a FIFO named on the command line cannot hang startup.
func HoldConfigFilePath(path string) {
	if data, ok := readRegularFile(path); ok {
		HoldConfigFile(data)
	}
}

// HoldProfileStoreFile holds the declared credential keys of the run profile
// store (credkit.HoldBody). Only a regular file is opened.
func HoldProfileStoreFile(path string) {
	if data, ok := readRegularFile(path); ok {
		holdBody(data)
	}
}

// HoldTLSKeyFile holds the private key the MCP server would serve TLS with: each
// PEM body line and the decoded key as hex, so neither its text nor its bytes in
// an encoding the kit reads can print. Armor and header lines are not the key,
// and holding them would withhold any PEM text. Its raw bytes are not held: the
// kit's startup check of a binary value is slow enough to stop the start. A key
// file is not a source contract section 1 names. Only a regular file is opened.
func HoldTLSKeyFile(path string) {
	data, ok := readRegularFile(path)
	if !ok {
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
		Hold("minted:tls-key", hex.EncodeToString(block.Bytes))
		rest = more
	}
}

func readRegularFile(path string) ([]byte, bool) {
	info, err := os.Stat(path)
	if err != nil || !info.Mode().IsRegular() {
		return nil, false
	}
	data, err := os.ReadFile(filepath.Clean(path))
	return data, err == nil
}

// StartupConfigFlag and StartupHomeFlag are the --config and --home values
// startup read the files with (credwire.credentialFiles): the last occurrence of
// each on the command line, or "". The kit took the allowed origin from that reading.
var StartupConfigFlag, StartupHomeFlag string

// startupRead is set when startup read the command line (SetStartupFlags). An
// in-process unit test never runs credkit.Start, so there is nothing to compare.
var startupRead bool

// SetStartupFlags records what startup read the files with.
func SetStartupFlags(config, home string) {
	StartupConfigFlag, StartupHomeFlag, startupRead = config, home, true
}

// CheckStartupFlags refuses a run whose parsed --config or --home is not what
// startup read. A flag that takes a value can swallow one (--select --config x),
// and then the allowed origin came from a file this run does not use.
func CheckStartupFlags(config, home string) error {
	if startupRead && (config != StartupConfigFlag || home != StartupHomeFlag) {
		return fmt.Errorf("--config or --home is also the value of another flag here, so it is not clear which file this run uses: write each once, as its own flag")
	}
	return nil
}

// CompactStdinBody puts a --stdin JSON body on one line before the kit reads it.
// Declarations.StdinFlags holds a body whole and line by line, and a line of "{"
// alone collides with public text and refuses the start, so a body from `jq .`
// would end the run. The credentials stay held: StdinFlags does that. Anything
// that is not one JSON document stays as it came.
func CompactStdinBody(args []string) {
	body := false
	for _, a := range args { // the kit's own test: every argument, either dash form
		name, _, _ := strings.Cut(a, "=")
		body = body || name == "--stdin" || name == "-stdin"
	}
	if !body {
		return
	}
	if fi, err := os.Stdin.Stat(); err != nil || fi.Mode()&os.ModeCharDevice != 0 {
		return
	}
	data, err := io.ReadAll(os.Stdin)
	if err != nil {
		return // the kit's own read of what is left fails closed
	}
	var one bytes.Buffer
	if json.Compact(&one, data) == nil {
		data = one.Bytes()
	}
	r, w, err := os.Pipe()
	if err != nil {
		return // os.Stdin is spent, so the command reads an empty body and refuses it
	}
	go func() {
		_, _ = w.Write(data)
		_ = w.Close()
	}()
	os.Stdin = r
}
