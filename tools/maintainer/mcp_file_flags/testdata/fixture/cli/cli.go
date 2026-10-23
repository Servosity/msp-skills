// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

// Package cli is the analyzer's self-test fixture. Every flag named want-*
// must be reported as naming a filesystem path (its value can start the path),
// every flag named comp-* as only ever being a path component under a
// directory the program chose, and every flag named not-* as neither. Each one
// exercises one data-flow shape.
package cli

import (
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io/fs"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
)

type fleetOpts struct {
	devices string
	device  string
}

func addFleetFlags(cmd *cobra.Command, o *fleetOpts) {
	cmd.Flags().StringVar(&o.devices, "want-struct-field", "", "devices file")
	cmd.Flags().StringVar(&o.device, "not-struct-sibling", "", "a device name")
}

func loadDevices(o *fleetOpts) error {
	fmt.Println("device", o.device)
	_, err := os.ReadFile(o.devices)
	return err
}

type pathValue struct{ p string }

func (v *pathValue) String() string     { return v.p }
func (v *pathValue) Set(s string) error { v.p = s; return nil }
func (v *pathValue) Type() string       { return "path" }

type opener interface{ Open(name string) error }
type diskOpener struct{}

func (diskOpener) Open(name string) error { _, err := os.Open(name); return err }

type request struct {
	Field string
	File  string
}

func encode(r request) error {
	fmt.Println(r.Field)
	_, err := os.Open(r.File)
	return err
}

func send(body any) error {
	if r, ok := body.(request); ok {
		return encode(r)
	}
	return nil
}

func normalize(s string) string { return strings.TrimSpace(s) }

func readHelper(path string) error {
	f, err := os.Open(filepath.Clean(path))
	if err == nil {
		_ = f.Close()
	}
	return err
}

func safeSegment(s string) string {
	var b strings.Builder
	for _, r := range s {
		if r >= 'a' && r <= 'z' {
			b.WriteRune(r)
		}
	}
	return b.String()
}

var homeDir string

// RootCmd builds the fixture command tree.
func RootCmd() *cobra.Command {
	root := &cobra.Command{Use: "fixture"}
	root.PersistentFlags().StringVar(&homeDir, "want-persistent", "", "root dir")

	var direct, at, helper, join, name, cacheKey, page, id, pick, sqlPath, prog, walk, mapped, built, decoded, multi, field, fsName string
	var joinHead, sprintfLead, sprintfName, concatName, dsn, runeLoop, safeComp string
	var slice []string
	opts := &fleetOpts{}
	run := &cobra.Command{
		Use: "run",
		RunE: func(cmd *cobra.Command, args []string) error {
			_, _ = os.ReadFile(direct)
			if strings.HasPrefix(at, "@") {
				_, _ = os.ReadFile(strings.TrimPrefix(at, "@"))
			}
			_ = readHelper(normalize(helper))
			fmt.Println(normalize(name))
			_ = os.WriteFile(filepath.Join(os.TempDir(), join+".json"), nil, 0o600)
			_ = os.MkdirAll(filepath.Join(joinHead, "cache"), 0o700)
			_ = os.WriteFile(fmt.Sprintf("%s.json", sprintfLead), nil, 0o600)
			_ = os.WriteFile(fmt.Sprintf("report-%s.html", sprintfName), nil, 0o600)
			_ = os.WriteFile(os.TempDir()+"/"+concatName, nil, 0o600)
			if db, err := sql.Open("sqlite", "file:"+dsn+"?mode=ro"); err == nil {
				_ = db.Close()
			}
			sum := sha256.Sum256([]byte(cacheKey))
			_ = os.WriteFile(filepath.Join(os.TempDir(), hex.EncodeToString(sum[:])), nil, 0o600)
			n, _ := strconv.Atoi(page)
			_ = os.WriteFile(filepath.Join(os.TempDir(), "page-"+strconv.Itoa(n)), nil, 0o600)
			if resp, err := http.Get("https://example.invalid/items/" + id); err == nil {
				_ = os.WriteFile(filepath.Join(os.TempDir(), resp.Header.Get("X-Name")), nil, 0o600)
			}
			fixed := map[string]string{"a": "/etc/fixture-a", "b": "/etc/fixture-b"}
			_, _ = os.ReadFile(fixed[pick])
			if db, err := sql.Open("sqlite", sqlPath); err == nil {
				_ = db.Close()
			}
			_ = exec.Command(prog).Run()
			_ = filepath.WalkDir(walk, func(string, fs.DirEntry, error) error { return nil })
			m := map[string]string{}
			m["k"] = mapped
			_, _ = os.ReadFile(m["k"])
			var sb strings.Builder
			sb.WriteString(built)
			_, _ = os.Open(sb.String())
			var rb strings.Builder
			for _, r := range runeLoop {
				rb.WriteRune(r)
			}
			_, _ = os.Open(rb.String())
			var cfg struct{ Path string }
			_ = json.Unmarshal([]byte(decoded), &cfg)
			_, _ = os.ReadFile(cfg.Path)
			for _, s := range slice {
				_, _ = os.Open(s)
			}
			_ = send(request{Field: field, File: multi})
			var o opener = diskOpener{}
			_ = o.Open(fsName)
			// A rune-filtered value placed as a path COMPONENT (the filter is
			// opaque to the analyzer, so it is a component finding, not gated).
			_ = os.MkdirAll(filepath.Join(os.TempDir(), safeSegment(safeComp)), 0o700)
			getter, _ := cmd.Flags().GetString("want-getter")
			_, _ = os.Stat(getter)
			_ = os.MkdirAll(homeDir, 0o700)
			return loadDevices(opts)
		},
	}
	f := run.Flags()
	f.StringVar(&direct, "want-direct", "", "read")
	f.StringVar(&at, "want-at-path", "", "inline JSON or @path")
	f.StringVar(&helper, "want-helper", "", "via helper")
	f.StringVar(&join, "comp-join", "", "file name inside a fixed directory")
	f.StringVar(&joinHead, "want-join-head", "", "directory that starts the path")
	f.StringVar(&sprintfLead, "want-sprintf-lead", "", "starts a formatted path")
	f.StringVar(&sprintfName, "comp-sprintf", "", "inside a formatted file name")
	f.StringVar(&concatName, "comp-concat", "", "after a directory and a separator")
	f.StringVar(&dsn, "want-scheme-dsn", "", "sqlite file inside a file: URI")
	f.StringVar(&name, "not-name", "", "printed only")
	f.StringVar(&cacheKey, "not-hashed", "", "hashed into a cache name")
	f.StringVar(&page, "not-number", "", "parsed as a number")
	f.StringVar(&id, "not-http", "", "sent to the API; the response names the file")
	f.StringVar(&pick, "not-map-key", "", "selects a fixed path")
	f.StringVar(&sqlPath, "want-sql-dsn", "", "sqlite file")
	f.StringVar(&prog, "want-exec", "", "program")
	f.StringVar(&walk, "want-walk", "", "directory")
	f.StringVar(&mapped, "want-map-value", "", "stored in a map")
	f.StringVar(&built, "want-builder", "", "built with a strings.Builder")
	f.StringVar(&runeLoop, "want-rune-loop", "", "copied rune by rune into a builder, then opened")
	f.StringVar(&safeComp, "comp-safe-segment", "", "rune-filtered, then used as a path component")
	f.StringVar(&decoded, "want-decoded", "", "JSON whose Path is read")
	f.StringSliceVar(&slice, "want-slice", nil, "files")
	f.StringVar(&multi, "want-multipart-file", "", "upload")
	f.StringVar(&field, "not-multipart-field", "", "form field")
	f.StringVar(&fsName, "want-interface", "", "opened through an interface")
	f.String("want-getter", "", "read with GetString")
	ptr := f.String("want-pointer", "", "returned pointer")
	f.Var(&pathValue{}, "want-value", "custom Value")
	addFleetFlags(run, opts)
	ptrCmd := &cobra.Command{Use: "ptr", RunE: func(*cobra.Command, []string) error {
		_, err := os.Create(*ptr)
		return err
	}}
	root.AddCommand(run, ptrCmd)
	return root
}

// Values set through a custom pflag.Value reach the disk here.
func useValue(v *pathValue) error {
	_, err := os.Open(v.p)
	return err
}

var _ = useValue
