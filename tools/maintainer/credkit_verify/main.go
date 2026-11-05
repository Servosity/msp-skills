// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

// Command credkit_verify checks that a connector wired the shared credential
// kit (tools/maintainer/credkit) by hand, completely. It verifies; it never
// infers a classification or edits code. Every site it cannot classify fails
// with the site named: a to-do for the connector, never a guess.
//
// For a connector whose cli/internal/credkit/ exists it checks:
//
//  1. The first statement of every main package's main() is credkit.Start,
//     given the connector's one credkit.Declarations literal.
//  2. Every reference to a sink primitive is routed through the kit or listed,
//     with a reason, in the connector's site ledger (credkit-sites.json).
//  3. Every string field of the connector's Config type and every environment
//     variable the code reads is classified in the Declarations.
//  4. Every file read (a config, credential-store or token-cache read among
//     them) is listed in the site ledger: a source Declarations.Files covers,
//     or not a credential source, each with a reason.
//  5. Each declared credential has at least one allowed origin.
//
// Usage (from the repository root):
//
//	go -C tools/maintainer/credkit_verify run . --slug <slug>
//	go -C tools/maintainer/credkit_verify run . --dir <path to skills/<slug>/cli>
//
// Exit 0: nothing unclassified, or the connector has no internal/credkit/
// (skipped with a note). Exit 1: findings. Exit 2: usage or load error.
package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// ledgerName is the site ledger, beside the connector's handfixes.json (outside
// cli/, so a reprint of cli/ never rewrites it).
const ledgerName = "credkit-sites.json"

// defaultTargets is every os/arch pair the release builds; CI runs the
// checker with this list. TestDefaultTargets asserts it in full.
const defaultTargets = "linux/amd64,linux/arm64,darwin/amd64,darwin/arm64,windows/amd64,windows/arm64"

func defaultTargetList() []string { return strings.Split(defaultTargets, ",") }

func main() {
	slug := flag.String("slug", "", "connector slug (skills/<slug>/cli)")
	dir := flag.String("dir", "", "connector cli directory (instead of --slug)")
	targets := flag.String("targets", defaultTargets, "comma-separated os/arch pairs to load the connector under (every pair the release builds)")
	flag.Parse()
	if (*slug == "") == (*dir == "") || flag.NArg() > 0 {
		fmt.Fprintln(os.Stderr, "usage: credkit_verify (--slug SLUG | --dir CLI_DIR) [--targets LIST]")
		os.Exit(2)
	}
	cli := *dir
	if *slug != "" {
		root, err := repoRoot()
		if err != nil {
			fmt.Fprintln(os.Stderr, "credkit_verify:", err)
			os.Exit(2)
		}
		cli = filepath.Join(root, "skills", *slug, "cli")
	}
	os.Exit(run(cli, strings.Split(*targets, ","), os.Stdout))
}

// repoRoot walks up from the working directory to the directory holding
// skills/ and tools/maintainer/.
func repoRoot() (string, error) {
	d, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for {
		if isDir(filepath.Join(d, "skills")) && isDir(filepath.Join(d, "tools", "maintainer")) {
			return d, nil
		}
		up := filepath.Dir(d)
		if up == d {
			return "", fmt.Errorf("no repository root (skills/ and tools/maintainer/) above the working directory")
		}
		d = up
	}
}

func isDir(p string) bool {
	fi, err := os.Stat(p)
	return err == nil && fi.IsDir()
}

// run checks one connector and prints every finding. It returns the exit code.
func run(cli string, targets []string, out io.Writer) int {
	if !isDir(cli) {
		fmt.Fprintf(out, "credkit_verify: %s is not a directory\n", cli)
		return 2
	}
	if !isDir(filepath.Join(cli, "internal", "credkit")) {
		fmt.Fprintf(out, "credkit_verify: %s: skipped, no internal/credkit/ (the connector has not adopted the kit)\n", cli)
		return 0
	}
	fs, err := verify(cli, targets)
	if err != nil {
		fmt.Fprintf(out, "credkit_verify: %s: %v\n", cli, err)
		return 2
	}
	for _, f := range fs {
		fmt.Fprintln(out, f)
	}
	if len(fs) > 0 {
		fmt.Fprintf(out, "credkit_verify: %s: %d unclassified site(s); each line names what would satisfy it\n", cli, len(fs))
		return 1
	}
	fmt.Fprintf(out, "credkit_verify: %s: ok, every sink, source, field and variable is routed or classified\n", cli)
	return 0
}
