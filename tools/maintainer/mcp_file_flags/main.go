// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

// Command mcp_file_flags reports every value a CLI caller chooses (a cobra
// flag, a positional argument, an MCP tool request) that can reach a host
// filesystem path in a connector module: an open, read, write, stat, mkdir,
// directory walk or exec, including a value whose "@path" prefix is expanded
// into a file read.
//
// It is the code-derived half of tools/maintainer/check_mcp_file_flags.py,
// which turns each reported flag into a live MCP assertion. The set is derived
// from data flow in the module's SSA form, never from flag names, so a
// regenerated connector that grows a new local-file flag is found on the PR
// that adds it.
//
// Usage:
//
//	go run . -dir skills/<slug>/cli > findings.json
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"go/types"
	"os"
	"path/filepath"
	"runtime/pprof"
	"sort"
	"strings"
	"time"

	"golang.org/x/tools/go/packages"
	"golang.org/x/tools/go/ssa"
	"golang.org/x/tools/go/ssa/ssautil"
)

// A finding is one source and the filesystem calls it reaches. Sinks are
// reached with the value at the START of the path, so the caller chooses the
// file or directory. ComponentSinks are reached only with the value AFTER a
// separator or other program-chosen text (filepath.Join(dir, name)): the
// caller chooses an entry inside a directory the program picked.
type finding struct {
	*source
	Sinks          []sinkSite `json:"sinks"`
	ComponentSinks []sinkSite `json:"component_sinks,omitempty"`
}

type report struct {
	Module    string `json:"module"`
	Packages  int    `json:"packages"`
	Functions int    `json:"functions"`
	Rounds    int    `json:"rounds"`
	Converged bool   `json:"converged"`
	Sources   int    `json:"sources"`
	// Flags name a path: the gated set.
	Flags []finding `json:"flags"`
	// FlagComponents only ever become a path component (reported, not gated).
	FlagComponents []finding `json:"flag_components"`
	Positional     []finding `json:"positional"`
	MCP            []finding `json:"mcp_requests"`
	MCPComponents  []finding `json:"mcp_request_components"`
	Other          []finding `json:"other"`
	Seconds        float64   `json:"seconds"`
	// Phase timings: package loading (go list and type-checking), SSA
	// construction, and the taint fixpoint.
	LoadSeconds  float64  `json:"load_seconds"`
	BuildSeconds float64  `json:"build_seconds"`
	SolveSeconds float64  `json:"solve_seconds"`
	Errors       []string `json:"errors,omitempty"`
}

func main() {
	dir := flag.String("dir", "", "connector module directory (skills/<slug>/cli)")
	maxRounds := flag.Int("max-rounds", 200, "fixpoint round limit")
	explainPos := flag.String("explain", "", "debug: print the objects carrying the source registered at file:line")
	cpuProfile := flag.String("cpuprofile", "", "debug: write a CPU profile here")
	flag.Parse()
	if *cpuProfile != "" {
		if f, err := os.Create(*cpuProfile); err == nil {
			_ = pprof.StartCPUProfile(f)
			defer pprof.StopCPUProfile()
		}
	}
	if *dir == "" {
		fmt.Fprintln(os.Stderr, "usage: mcp_file_flags -dir skills/<slug>/cli")
		os.Exit(2)
	}
	start := time.Now()
	rep, err := analyzeModule(*dir, *maxRounds, *explainPos)
	if err != nil {
		fmt.Fprintln(os.Stderr, "mcp_file_flags:", err)
		os.Exit(1)
	}
	rep.Seconds = time.Since(start).Seconds()
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", " ")
	if err := enc.Encode(rep); err != nil {
		os.Exit(1)
	}
	if !rep.Converged || len(rep.Errors) > 0 {
		os.Exit(3)
	}
}

func analyzeModule(dir string, maxRounds int, explainPos string) (*report, error) {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return nil, err
	}
	cfg := &packages.Config{
		Mode: packages.NeedName | packages.NeedFiles | packages.NeedCompiledGoFiles |
			packages.NeedImports | packages.NeedTypes | packages.NeedTypesSizes |
			packages.NeedSyntax | packages.NeedTypesInfo | packages.NeedModule,
		Dir:   abs,
		Tests: false,
		Env:   append(os.Environ(), "GOFLAGS=-mod=readonly", "CGO_ENABLED=0"),
	}
	t0 := time.Now()
	pkgs, err := packages.Load(cfg, "./...")
	if err != nil {
		return nil, err
	}
	rep := &report{}
	for _, p := range pkgs {
		for _, e := range p.Errors {
			rep.Errors = append(rep.Errors, e.Error())
		}
		if p.Module != nil && rep.Module == "" {
			rep.Module = p.Module.Path
		}
	}
	if len(rep.Errors) > 0 {
		return rep, nil
	}
	rep.Packages = len(pkgs)
	rep.LoadSeconds = time.Since(t0).Seconds()
	t1 := time.Now()

	prog := ssa.NewProgram(pkgs[0].Fset, ssa.InstantiateGenerics)
	inMod := map[*ssa.Package]bool{}
	created := map[*types.Package]bool{}
	for _, p := range pkgs {
		created[p.Types] = true
	}
	for _, p := range pkgs {
		inMod[prog.CreatePackage(p.Types, p.Syntax, p.TypesInfo, true)] = true
	}
	// Every dependency is created from type information only: its functions
	// have no bodies, so the analysis applies the rules in specs.go to them.
	var createDeps func(*types.Package)
	createDeps = func(tp *types.Package) {
		for _, imp := range tp.Imports() {
			if created[imp] {
				continue
			}
			created[imp] = true
			createDeps(imp)
			prog.CreatePackage(imp, nil, nil, true)
		}
	}
	for _, p := range pkgs {
		createDeps(p.Types)
	}
	prog.Build()

	var funcs []*ssa.Function
	for fn := range ssautil.AllFunctions(prog) {
		if fn.Blocks == nil {
			continue
		}
		if fn.TypeParams().Len() > 0 && len(fn.TypeArgs()) == 0 {
			continue // generic origin; its instantiations are analysed
		}
		funcs = append(funcs, fn)
	}
	sort.Slice(funcs, func(i, j int) bool {
		if funcs[i].Pos() != funcs[j].Pos() {
			return funcs[i].Pos() < funcs[j].Pos()
		}
		return funcs[i].String() < funcs[j].String()
	})
	rep.Functions = len(funcs)

	rep.BuildSeconds = time.Since(t1).Seconds()
	t2 := time.Now()
	e := newEngine(prog, inMod, funcs)
	e.seedEntryPoints()
	if explainPos != "" {
		e.explainSetup(explainPos)
	}
	rep.Rounds = e.run(maxRounds)
	rep.Converged = e.converged
	rep.SolveSeconds = time.Since(t2).Seconds()
	rep.Sources = len(e.sources)
	if explainPos != "" {
		e.explain(os.Stderr, explainPos)
	}

	trim := func(p string) string {
		if rel, err := filepath.Rel(abs, p); err == nil && !strings.HasPrefix(rel, "..") {
			return filepath.ToSlash(rel)
		}
		return p
	}
	trimSites := func(sinks []sinkSite) []sinkSite {
		out := append([]sinkSite(nil), sinks...)
		for i := range out {
			out[i].Pos = trim(out[i].Pos)
			if parts := strings.SplitN(out[i].Via, " ", 2); len(parts) == 2 {
				out[i].Via = trim(parts[0]) + " " + parts[1]
			} else if out[i].Via != "" {
				out[i].Via = trim(out[i].Via)
			}
		}
		return out
	}
	byLead := map[int]*finding{}
	var leads []int
	for _, id := range e.sortedFindings() {
		lead := sourceLead(id)
		f := byLead[lead]
		if f == nil {
			s := e.sources[(lead-symbolicSlots)/2]
			s.Pos = trim(s.Pos)
			f = &finding{source: s}
			byLead[lead] = f
			leads = append(leads, lead)
		}
		if isComponent(id) {
			f.ComponentSinks = trimSites(e.findings[id])
		} else {
			f.Sinks = trimSites(e.findings[id])
		}
	}
	sort.Ints(leads)
	for _, lead := range leads {
		f := *byLead[lead]
		path := len(f.Sinks) > 0
		switch f.Kind {
		case "flag", "flag-getter":
			if path {
				rep.Flags = append(rep.Flags, f)
			} else {
				rep.FlagComponents = append(rep.FlagComponents, f)
			}
		case "positional":
			rep.Positional = append(rep.Positional, f)
		case "mcp-request":
			if path {
				rep.MCP = append(rep.MCP, f)
			} else {
				rep.MCPComponents = append(rep.MCPComponents, f)
			}
		default:
			if path {
				rep.Other = append(rep.Other, f)
			}
		}
	}
	return rep, nil
}
