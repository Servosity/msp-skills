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
// A unit request may carry "units", the writes of one stream between pauses,
// in order: when the whole unit is clean, the kit's stream check runs over
// them (credkit.StreamWithheld) and a withheld write is a leak.
//
// A request {"acquire": {"decl": {...}, "env": [...], "args": [...],
// "mcp_in": ..., "stdin": ..., "exchanges": [...]}} asks for the kit's held set of one run
// (credkit.Acquire), answered {"verdict": "held", "held": [...],
// "recorded": n, "unknown": bool}.
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
	Units   []string `json:"units"` // a stream's writes, in order
	Request bool     `json:"request"`
	Public  []string `json:"public"`
	Acquire *acquire `json:"acquire"`
}

// acquire asks for the kit's held set of one run (credkit.Acquire). Every
// value is base64.
type acquire struct {
	Decl      decl       `json:"decl"`
	Env       []string   `json:"env"`
	Args      []string   `json:"args"`
	MCPIn     string     `json:"mcp_in"` // what the MCP server read on stdin
	Stdin     string     `json:"stdin"`  // what the process was given on stdin
	Exchanges []exchange `json:"exchanges"`
}

// exchange is one request sent or one response delivered, as sent.
type exchange struct {
	Response bool        `json:"response"`
	Method   string      `json:"method"`
	Status   int         `json:"status"`
	URL      string      `json:"url"`
	Header   [][2]string `json:"header"`
	Trailer  [][2]string `json:"trailer"`
	Body     string      `json:"body"`
}

// decl is the part of credkit.Declarations the list reads and sends (its
// Files function has no JSON form).
type decl struct {
	SecretEnv, ConfigFields, StoredRecords, CredentialFlags, StdinFlags []string
	CredentialHeaders, CredentialFields, ResponseFields, MCPArguments   []string
	Identifiers                                                         []credkit.Identifier
	BaseURL                                                             credkit.BaseURL
}

func (d decl) kit() credkit.Declarations {
	return credkit.Declarations{SecretEnv: d.SecretEnv, ConfigFields: d.ConfigFields, StoredRecords: d.StoredRecords,
		CredentialFlags: d.CredentialFlags, StdinFlags: d.StdinFlags, CredentialHeaders: d.CredentialHeaders,
		CredentialFields: d.CredentialFields, ResponseFields: d.ResponseFields, MCPArguments: d.MCPArguments,
		Identifiers: d.Identifiers, BaseURL: d.BaseURL}
}

type answer struct {
	Verdict  string   `json:"verdict"`
	Clause   string   `json:"clause,omitempty"`
	Public   []bool   `json:"public,omitempty"`
	Held     []string `json:"held,omitempty"`
	Recorded int      `json:"recorded,omitempty"`
	Unknown  bool     `json:"unknown,omitempty"`
}

func unb64(vs []string) ([]string, error) {
	out := make([]string, len(vs))
	for i, v := range vs {
		b, err := base64.StdEncoding.DecodeString(v)
		if err != nil {
			return nil, err
		}
		out[i] = string(b)
	}
	return out, nil
}

func acquired(a *acquire) (answer, error) {
	env, err := unb64(a.Env)
	if err != nil {
		return answer{}, err
	}
	args, err := unb64(a.Args)
	if err != nil {
		return answer{}, err
	}
	var ex []credkit.Exchange
	for _, x := range a.Exchanges {
		b, err := base64.StdEncoding.DecodeString(x.Body)
		if err != nil {
			return answer{}, err
		}
		pairs := func(kv [][2]string) ([][2]string, error) {
			out := make([][2]string, len(kv))
			for i, p := range kv {
				v, err := unb64([]string{p[0], p[1]})
				if err != nil {
					return nil, err
				}
				out[i] = [2]string{v[0], v[1]}
			}
			return out, nil
		}
		h, err := pairs(x.Header)
		if err != nil {
			return answer{}, err
		}
		t, err := pairs(x.Trailer)
		if err != nil {
			return answer{}, err
		}
		ex = append(ex, credkit.Exchange{Response: x.Response, Method: x.Method, Status: x.Status, URL: x.URL,
			Header: h, Trailer: t, Body: b})
	}
	mcpIn, err := base64.StdEncoding.DecodeString(a.MCPIn)
	if err != nil {
		return answer{}, err
	}
	stdin, err := base64.StdEncoding.DecodeString(a.Stdin)
	if err != nil {
		return answer{}, err
	}
	held, rec, unknown, err := credkit.Acquire(a.Decl.kit(), env, args, mcpIn, stdin, ex)
	if err != nil {
		return answer{}, err
	}
	out := answer{Verdict: "held", Held: []string{}, Recorded: rec, Unknown: unknown}
	for _, v := range held {
		out.Held = append(out.Held, base64.StdEncoding.EncodeToString([]byte(v)))
	}
	return out, nil
}

func main() {
	// The kit writes its own notices (a value recorded, not held) to
	// os.Stderr from Acquire; they are not this command's output.
	errOut := os.Stderr
	if null, err := os.OpenFile(os.DevNull, os.O_WRONLY, 0); err == nil {
		os.Stderr = null
	}
	if err := serve(os.Stdin, os.Stdout); err != nil {
		fmt.Fprintln(errOut, "credmatch:", err)
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
		if q.Acquire != nil {
			a, err := acquired(q.Acquire)
			if err != nil {
				return fmt.Errorf("line %d: acquire: %v", n, err)
			}
			if err := enc.Encode(a); err != nil {
				return err
			}
			if err := w.Flush(); err != nil {
				return err
			}
			continue
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
		if v == "clean" && len(q.Units) > 1 {
			units := make([][]byte, len(q.Units))
			for i, u := range q.Units {
				if units[i], err = base64.StdEncoding.DecodeString(u); err != nil {
					return fmt.Errorf("line %d: units: %v", n, err)
				}
			}
			if i := credkit.StreamWithheld(held, units); i >= 0 {
				v, c = "leak", fmt.Sprintf("3 (Streams: write %d of %d withheld)", i+1, len(units))
			}
		}
		if err := enc.Encode(answer{Verdict: v, Clause: c}); err != nil {
			return err
		}
		if err := w.Flush(); err != nil {
			return err
		}
	}
}
