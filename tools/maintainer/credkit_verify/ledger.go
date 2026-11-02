// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package main

// The site ledger, skills/<slug>/credkit-sites.json: the connector's hand
// classification of each code site the checker cannot see routed. An entry
// names a site by file, enclosing function and primitive, and the number of
// such references in that function, so a reprint that adds one more fails
// until the ledger is updated, and an entry that matches nothing is stale.
//
//	{
//	  "config_types": ["internal/config.Config"],
//	  "sites": [
//	    {"file": "internal/cli/export.go", "func": "writeExport", "what": "os.WriteFile",
//	     "count": 1, "kind": "exempt", "reason": "writes the public spec, never a response"},
//	    {"file": "internal/config/config.go", "func": "Load", "what": "os.ReadFile",
//	     "count": 1, "kind": "source", "via": "internal/config.Path",
//	     "reason": "the config file Declarations.Files returns"}
//	  ]
//	}
//
// kind "source" (a file read only) says the read is a credential source the
// Declarations' Files function names, and "via" proves it: Files reaches that
// function, and the read's path is a call to it; kind "exempt" says the site is not a
// credential sink or source, or is routed in a way the checker cannot see.
// An environment read whose variable name the checker cannot resolve needs
// "names": every variable it may read (each is then classified like any
// other read); an empty list says it reads no single variable.

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"sort"
	"strings"
)

type ledger struct {
	ConfigTypes []string `json:"config_types"`
	Sites       []entry  `json:"sites"`
}

type entry struct {
	File   string    `json:"file"`
	Func   string    `json:"func"`
	What   string    `json:"what"`
	Count  int       `json:"count"`
	Kind   string    `json:"kind"`
	Reason string    `json:"reason"`
	Names  *[]string `json:"names"`
	Via    string    `json:"via"` // kind "source": the function Files and the read's path both come from
}

// loadLedger reads the ledger. A missing ledger is an empty one; one that does
// not parse, or carries a field the checker does not know, is an error.
func loadLedger(path string) (*ledger, error) {
	b, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return &ledger{}, nil
	}
	if err != nil {
		return nil, err
	}
	var l ledger
	d := json.NewDecoder(bytes.NewReader(b))
	d.DisallowUnknownFields()
	if err := d.Decode(&l); err != nil {
		return nil, fmt.Errorf("%s: %v", path, err)
	}
	return &l, nil
}

type siteKey struct{ file, fn, what string }

// apply clears each group of site findings its ledger entry lists exactly and
// returns what is left, plus a finding for each entry that is malformed, does
// not match its sites, or matches none.
func apply(fs []Finding, l *ledger) []Finding {
	groups := map[siteKey][]Finding{}
	var out []Finding
	for _, f := range fs {
		if f.Site {
			k := siteKey{f.File, f.Func, f.What}
			groups[k] = append(groups[k], f)
		} else {
			out = append(out, f)
		}
	}
	seen := map[siteKey]bool{}
	for _, e := range l.Sites {
		k := siteKey{e.File, e.Func, e.What}
		bad := func(msg string) {
			out = append(out, Finding{Rule: 0, File: ledgerName, What: "ledger", Msg: fmt.Sprintf("entry {file %q, func %q, what %q}: %s", e.File, e.Func, e.What, msg), Fix: "correct or remove the entry"})
		}
		switch {
		case seen[k]:
			bad("listed twice")
			continue
		case e.File == "" || e.Func == "" || e.What == "":
			bad("file, func and what are all required")
			continue
		case strings.TrimSpace(e.Reason) == "":
			bad("has no reason")
			continue
		case e.Kind != "exempt" && e.Kind != "source":
			bad(`kind must be "exempt" or "source"`)
			continue
		}
		seen[k] = true
		g, ok := groups[k]
		if !ok {
			bad("matches no site (stale: the code no longer has it)")
			continue
		}
		if e.Count != len(g) {
			bad(fmt.Sprintf("count is %d but the function has %d such site(s)", e.Count, len(g)))
			continue
		}
		if e.Kind == "source" && !g[0].Read {
			bad(`kind "source" is for a file read only`)
			continue
		}
		if e.Kind == "source" && e.Via == "" {
			bad(`kind "source" needs "via": the package-level function (as "<package path in the module>.<Func>") that both Declarations.Files and the read's path call`)
			continue
		}
		if g[0].Env && e.Names == nil {
			bad(`an environment read the checker cannot resolve needs "names"`)
			continue
		}
		delete(groups, k)
	}
	for _, g := range groups {
		out = append(out, g...)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].less(out[j]) })
	return out
}

// ledgerNames is every variable name the ledger's environment entries list.
func ledgerNames(l *ledger) []string {
	var out []string
	for _, e := range l.Sites {
		if e.Names != nil {
			out = append(out, *e.Names...)
		}
	}
	return out
}
