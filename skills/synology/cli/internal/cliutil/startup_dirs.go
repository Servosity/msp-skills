// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

// Hand-written (NOT generated; handfixes.json credkit-wiring).

package cliutil

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
)

// StartupKindDirs is every directory kind can resolve to for this process:
// each rung ResolveKindDir walks (per-kind variable, --home, SYNOLOGY_HOME,
// XDG, platform default), not only the one that wins, without and with each
// of homes (the --home values the arguments name) applied. Holding the files
// of every rung is a superset of what the run reads. Nothing is printed: a
// relative override is skipped here as ResolveKindDir skips it, and
// ResolveKindDir still warns about it when the run resolves the kind.
func StartupKindDirs(kind PathKind, homes []string) []string {
	var dirs []string
	for _, home := range append([]string{""}, homes...) {
		for _, dir := range kindRungs(kind, home) {
			if !slices.Contains(dirs, dir) {
				dirs = append(dirs, dir)
			}
		}
	}
	return dirs
}

// StartupKindDir is the directory kind resolves to under home ("" for no --home):
// the rung ResolveKindDir takes, without printing. "" when home is one the run refuses.
func StartupKindDir(kind PathKind, home string) string {
	if rungs := kindRungs(kind, home); len(rungs) > 0 {
		return rungs[0]
	}
	return ""
}

// kindRungs is kind's rungs under home, in ResolveKindDir's order.
func kindRungs(kind PathKind, home string) []string {
	clean := ""
	if home != "" {
		c, ok := CleanPathOverride(home)
		if !ok {
			return nil
		}
		clean = c
	}
	quiet := func(name string) (string, bool) {
		raw := strings.TrimSpace(os.Getenv(name))
		if name == "" || raw == "" {
			return "", false
		}
		return CleanPathOverride(raw)
	}
	var dirs []string
	add := func(dir string) {
		if dir != "" {
			dirs = append(dirs, dir)
		}
	}
	restore := setHomeOverride(clean)
	defer restore()
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
	return dirs
}
