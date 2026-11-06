// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

// Hand-written (NOT generated; handfixes.json credkit-wiring).
// A config, credentials or run profile file read after the kit's startup read
// (the MCP server reloads the config on every tool call, and a store written
// since) is held with the kit's own readers before it is parsed: its credential
// fields, every word of a file that does not parse, and the password and query
// values of its base_url. No reader of this connector's own: credkit.HoldBody
// and credkit.HoldURL are the kit's.

package cliutil

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"autotask-pp-cli/internal/credkit"

	"github.com/pelletier/go-toml/v2"
)

// Hold is credkit.Hold. Production code never reassigns it. In-process unit tests
// may, to a no-op: a held value lasts for the process and the kit withholds
// whatever carries it. The built binaries' leak tests (internal/credwire) prove
// the real thing.
var Hold = credkit.Hold

// MaskedResponseFields are the spec's credential members, replaced by name at the
// client (client.maskCredentialFields, handfixes.json credential-fields-withheld)
// so a listing that carries one stays usable: a webhook's secretKey, a
// client-portal user's password, an integration vendor insight's or widget's
// secret. The kit holds the same names as ResponseFields.
var MaskedResponseFields = []string{"secretKey", "password", "secret"}

// SecretFieldWithheld replaces a masked field's value.
const SecretFieldWithheld = "[withheld]"

// HoldConfigFile holds the credentials in a config or credentials file.
func HoldConfigFile(data []byte) {
	credkit.HoldBody(data)
	var parsed struct {
		BaseURL string `toml:"base_url"`
	}
	if toml.Unmarshal(data, &parsed) == nil {
		credkit.HoldURL("userinfo", parsed.BaseURL)
	}
}

// HoldConfigFilePath is HoldConfigFile on a file. Only a regular file is opened,
// so a FIFO named on the command line cannot hang startup.
func HoldConfigFilePath(path string) {
	if data, err := readRegularFile(path); err == nil {
		HoldConfigFile(data)
	}
}

// HoldProfileStore holds a run profile store's credentials: every value under a
// declared credential name, each duplicate key included (credkit.HoldBody). A
// store that is not JSON is refused by its loader, so it is not read here at
// startup, where every word of it would be held.
func HoldProfileStore(data []byte) {
	if json.Valid(data) {
		credkit.HoldBody(data)
	}
}

// HoldProfileStoreFile is HoldProfileStore on a file. Only a regular file is opened.
func HoldProfileStoreFile(path string) {
	if data, err := readRegularFile(path); err == nil {
		HoldProfileStore(data)
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
	return os.ReadFile(filepath.Clean(path))
}

// StartupConfigFlag and StartupHomeFlag are the --config and --home values startup
// read the files with (credwire.credentialFiles): the last occurrence of each on the
// command line, or "". The kit took the allowed origin from that reading.
var StartupConfigFlag, StartupHomeFlag string

// CheckStartupFlags refuses a run whose parsed --config or --home is not what startup
// read. A flag that takes a value can swallow one (--select --config x), and then the
// allowed origin came from a file this run does not use.
func CheckStartupFlags(config, home string) error {
	if config != StartupConfigFlag || home != StartupHomeFlag {
		return fmt.Errorf("--config or --home is also the value of another flag here, so it is not clear which file this run uses: write each once, as its own flag")
	}
	return nil
}

// CompactStdinBody puts a --stdin JSON body on one line before the kit reads it.
// Declarations.StdinFlags holds a body whole and line by line, and a line of "{" alone
// collides with public text and refuses the start, so a body from `jq .` would end
// the run. The credentials stay held: StdinFlags does that. Anything that is not one
// JSON document stays as it came.
func CompactStdinBody(args []string) {
	body := false
	for _, a := range args {
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
