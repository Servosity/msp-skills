// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

//go:build credmatch

// Command credmatch is the leak test list's one call into the kit's matcher
// (contract 8). It is built only by class_secret_corpus.py, from a temporary
// copy of the kit with kit/export.go added, and deleted after the run.
//
// Each stdin line is one JSON request:
//
//	{"held": ["<base64 of a held value>", ...], "unit": "<base64>", "request": false}
//
// and each stdout line its answer, in order:
//
//	{"verdict": "clean" | "leak" | "exhausted", "clause": "2(a)" | "2(b)" | "2(c)" | ...}
//
// A request {"public": ["<base64>", ...]} instead asks the kit's public-text
// rule for values acquired while running, one answer per value:
//
//	{"public": [true, false, ...]}
//
// A line it cannot read ends the command with status 2, so the caller fails
// the run rather than taking a missing verdict as clean.
package main

import (
	"bufio"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"slices"

	"credkit"
)

type request struct {
	Held    []string `json:"held"`
	Unit    string   `json:"unit"`
	Request bool     `json:"request"`
	Public  []string `json:"public"`
}

type answer struct {
	Verdict string `json:"verdict"`
	Clause  string `json:"clause,omitempty"`
	Public  []bool `json:"public,omitempty"`
}

func main() {
	if err := serve(os.Stdin, os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, "credmatch:", err)
		os.Exit(2)
	}
}

func serve(in io.Reader, out io.Writer) error {
	r := bufio.NewReaderSize(in, 1<<20)
	w := bufio.NewWriter(out)
	enc := json.NewEncoder(w)
	for n := 1; ; n++ {
		line, err := r.ReadBytes('\n')
		if len(line) == 0 && err == io.EOF {
			return nil
		}
		if err != nil && err != io.EOF {
			return err
		}
		var q request
		if err := json.Unmarshal(line, &q); err != nil {
			return fmt.Errorf("line %d: %v", n, err)
		}
		if q.Public != nil {
			pub := make([]bool, len(q.Public))
			for i, v := range q.Public {
				b, err := base64.StdEncoding.DecodeString(v)
				if err != nil {
					return fmt.Errorf("line %d: public value %d: %v", n, i, err)
				}
				pub[i] = credkit.PublicAsWritten(string(b))
			}
			if err := enc.Encode(answer{Public: pub}); err != nil {
				return err
			}
			if err := w.Flush(); err != nil {
				return err
			}
			continue
		}
		held := make([]string, 0, len(q.Held))
		for i, h := range q.Held {
			b, err := base64.StdEncoding.DecodeString(h)
			if err != nil {
				return fmt.Errorf("line %d: held value %d: %v", n, i, err)
			}
			held = append(held, string(b))
		}
		slices.Sort(held)
		unit, err := base64.StdEncoding.DecodeString(q.Unit)
		if err != nil {
			return fmt.Errorf("line %d: unit: %v", n, err)
		}
		v, c := credkit.MatchUnit(held, unit, q.Request)
		if err := enc.Encode(answer{Verdict: v, Clause: c}); err != nil {
			return err
		}
		if err := w.Flush(); err != nil {
			return err
		}
	}
}
