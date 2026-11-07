// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

// Hand-written (NOT generated; handfixes.json credkit-wiring).
// What the connector hands the credential kit itself, with the kit's own
// readers (credkit.HoldBody, credkit.HoldURL): a config or credentials file
// read after the kit's startup read (the MCP server reloads the file on every
// tool call, and a store written since), the run profile store, a token typed
// at a prompt, the MCP server's TLS key. No reader of its own and no scan of
// free text: contract section 1 is a closed list.

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

	"domotz-pp-cli/internal/credkit"

	"github.com/pelletier/go-toml/v2"
)

// Hold, HoldBody and HoldURL are the kit's. Production code never reassigns
// them. In-process unit tests do, to no-ops: a held value lasts for the
// process and the kit withholds whatever carries it, so one test's fixture
// would withhold the next test's file. The built binaries' leak tests
// (internal/credwire) prove the real thing.
var (
	Hold     = credkit.Hold
	HoldBody = credkit.HoldBody
	HoldURL  = credkit.HoldURL
)

// HoldConfigFile holds the credentials in a config or credentials file before
// it is parsed: its credential fields and [headers] values (credkit.HoldBody,
// which holds every word of a file that does not parse), and the password and
// query values of its base_url.
func HoldConfigFile(data []byte) {
	HoldBody(data)
	var parsed struct {
		BaseURL string `toml:"base_url"`
	}
	if toml.Unmarshal(data, &parsed) == nil && parsed.BaseURL != "" {
		HoldURL("userinfo", parsed.BaseURL)
	}
}

// HoldConfigFilePath holds every credential of a config or credential file
// this run does not use, by the same rule as a file it reads. Only a regular
// file is opened, so a FIFO named on the command line cannot hang startup.
func HoldConfigFilePath(path string) {
	if data, err := readRegularFile(path); err == nil {
		HoldConfigFile(data)
	}
}

// HoldProfileStoreFile holds what contract section 1 names in a run profile
// store (profiles.json): a value under a credential flag's or field's name,
// every duplicate key included (credkit.HoldBody reads every member), and the
// password and every query value of a value under base-url (credkit.HoldURL,
// source profile:base-url, which the Declarations send nowhere). A profile's
// name, description and other values are data. Only a regular file is opened.
func HoldProfileStoreFile(path string) {
	if data, err := readRegularFile(path); err == nil {
		HoldBody(data)
		holdProfileBaseURLs(data)
	}
}

// holdProfileBaseURLs hands every string under a base-url member, at any depth
// and every duplicate included, to the kit's URL reader. It locates values
// only; what is held in them is the kit's rule. A store that stops parsing
// gives up what comes before the error.
func holdProfileBaseURLs(data []byte) {
	dec := json.NewDecoder(bytes.NewReader(data))
	var key string
	var inObject []bool
	expectKey := false
	for {
		tok, err := dec.Token()
		if err != nil {
			return
		}
		switch t := tok.(type) {
		case json.Delim:
			switch t {
			case '{':
				inObject = append(inObject, true)
				expectKey = true
			case '[':
				inObject = append(inObject, false)
				expectKey = false
			default:
				inObject = inObject[:len(inObject)-1]
				expectKey = len(inObject) > 0 && inObject[len(inObject)-1]
			}
			continue
		case string:
			if expectKey {
				key, expectKey = t, false
				continue
			}
			if strings.EqualFold(key, "base-url") {
				HoldURL("profile:base-url", t)
			}
		}
		key = ""
		expectKey = len(inObject) > 0 && inObject[len(inObject)-1]
	}
}

// HoldTLSKeyFile holds the private key the MCP server would serve TLS with:
// each PEM body line and the decoded key as hex, so neither its text nor its
// bytes in an encoding the kit reads can be printed. Armor and header lines
// are not the key. Only a regular file is opened.
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
		Hold("minted:tls-key", hex.EncodeToString(block.Bytes))
		rest = more
	}
}

// StartupConfigFlag and StartupHomeFlag are the --config and --home values
// startup read the files with (credwire.credentialFiles): the last occurrence
// of each on the command line, or "". The kit took the allowed origin from
// that reading.
var StartupConfigFlag, StartupHomeFlag string

// CheckStartupFlags refuses a run whose parsed --config or --home is not what
// startup read. A flag that takes a value can swallow one (--select --config
// x), and then the allowed origin came from a file this run does not use.
func CheckStartupFlags(config, home string) error {
	if config != StartupConfigFlag || home != StartupHomeFlag {
		return fmt.Errorf("--config or --home is also the value of another flag here, so it is not clear which file this run uses: write each once, as its own flag")
	}
	return nil
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

// CompactStdinBody puts a --stdin JSON body on one line before the kit reads
// it. Declarations.StdinFlags holds a body whole and line by line, and a line
// of "{" alone collides with public text and refuses the start, so a body from
// `jq .` would end the run. The credentials stay held: StdinFlags does that.
// Anything that is not one JSON document stays as it came.
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
