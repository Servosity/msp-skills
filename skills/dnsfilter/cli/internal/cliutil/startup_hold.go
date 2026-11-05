// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

// Hand-written (NOT generated; handfixes.json credkit-wiring): the directories
// the credential kit's startup discovery reads (internal/credwire) and the
// parse-error text that names no value of the file.

package cliutil

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/pelletier/go-toml/v2"
)

// StartupKindDirs is every directory kind can resolve to for this process:
// each rung ResolveKindDir walks (per-kind variable, --home, DNSFILTER_HOME,
// XDG, platform default), not only the one that wins, with each of homes (the
// --home values the arguments name, the last first: it is the one the parser
// keeps) applied and then without them. Holding the files
// of every rung is a superset of what the run reads. Nothing is printed: a
// relative override is skipped here as ResolveKindDir skips it, and
// ResolveKindDir still warns about it when the run resolves the kind.
func StartupKindDirs(kind PathKind, homes []string) []string {
	var dirs []string
	add := func(dir string) {
		if dir != "" && !slices.Contains(dirs, dir) {
			dirs = append(dirs, dir)
		}
	}
	quiet := func(name string) (string, bool) {
		raw := strings.TrimSpace(os.Getenv(name))
		if name == "" || raw == "" {
			return "", false
		}
		return CleanPathOverride(raw)
	}
	rungs := append(slices.Clone(homes), "")
	slices.Reverse(rungs[:len(homes)])
	for _, home := range rungs {
		clean := ""
		if home != "" {
			c, ok := CleanPathOverride(home)
			if !ok {
				continue
			}
			clean = c
		}
		restore := setHomeOverride(clean)
		if dir, ok := quiet(kindEnvVar(kind)); ok {
			add(dir)
		}
		if clean != "" {
			add(filepath.Join(clean, kindName(kind)))
		}
		if dir, ok := quiet(envPrefix + "_HOME"); ok {
			add(filepath.Join(dir, kindName(kind)))
		}
		if dir, ok := quiet(xdgEnvVar(kind)); ok {
			add(filepath.Join(dir, appName))
		}
		if base, err := defaultBase(kind); err == nil {
			add(filepath.Join(base, appName))
		}
		restore()
	}
	return dirs
}

// TOMLParseError describes a configuration or credentials file TOML rejected
// by position only. The parser's own text can quote the file (a duplicate key
// is named, a bad value is repeated, a TextUnmarshaler error quotes its
// input), and the file holds credentials, so none of that text is kept and
// the parser's error is not reachable through Unwrap.
func TOMLParseError(err error) error {
	if err == nil {
		return nil
	}
	var decodeErr *toml.DecodeError
	if errors.As(err, &decodeErr) {
		row, col := decodeErr.Position()
		return &tomlParseError{row: row, col: col}
	}
	// go-toml reports a repeated key ("toml: key <name> is already defined")
	// without a position, and the name can be a secret pasted as a key.
	if strings.HasSuffix(strings.TrimSpace(err.Error()), "is already defined") {
		return &tomlParseError{duplicate: true}
	}
	return &tomlParseError{}
}

type tomlParseError struct {
	row, col  int
	duplicate bool
}

func (e *tomlParseError) Error() string {
	switch {
	case e.row > 0:
		return fmt.Sprintf("invalid TOML at line %d, column %d", e.row, e.col)
	case e.duplicate:
		return "invalid TOML: a key or table is defined twice"
	}
	return "invalid TOML: a value has the wrong type or is out of range"
}
