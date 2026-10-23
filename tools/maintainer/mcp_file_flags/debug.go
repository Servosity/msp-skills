// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package main

import (
	"fmt"
	"go/types"
	"io"
	"sort"
	"strings"

	"golang.org/x/tools/go/ssa"
)

// explain prints every object that carries the source registered at pos
// (a "file:line" suffix), for debugging a surprising finding.
func (e *engine) explain(w io.Writer, pos string) {
	for _, s := range e.sources {
		if !strings.HasSuffix(s.Pos, pos) {
			continue
		}
		fmt.Fprintf(w, "source %d %s %s %s\n", s.ID, s.Kind, s.Name, s.Pos)
		var lines []string
		for k, t := range e.objTags {
			if !t.has(s.ID) {
				continue
			}
			lines = append(lines, "  obj "+e.describeObj(k)+"\n      <- "+e.why[k])
		}
		for fn, seeds := range e.paramSeeds {
			for i, t := range seeds {
				if t.has(s.ID) {
					lines = append(lines, fmt.Sprintf("  seed %s param %d", fn, i))
				}
			}
		}
		for fn, fv := range e.fvTags {
			for i, t := range fv {
				if t.has(s.ID) {
					lines = append(lines, fmt.Sprintf("  fv %s %d", fn, i))
				}
			}
		}
		for fn, sum := range e.sums {
			if unionAll(sum.ret).has(s.ID) {
				lines = append(lines, fmt.Sprintf("  ret %s", fn))
			}
		}
		sort.Strings(lines)
		for _, l := range lines {
			fmt.Fprintln(w, l)
		}
	}
}

func (e *engine) describeObj(k objKey) string {
	switch x := k.(type) {
	case *types.Var:
		return fmt.Sprintf("field %s (%s)", x.Name(), e.pos(x.Pos()))
	case *ssa.Global:
		return "global " + x.String()
	case ssa.Instruction:
		v, _ := k.(ssa.Value)
		name := ""
		if v != nil {
			name = v.Name() + " = " + v.String()
		}
		return fmt.Sprintf("%T %s in %s (%s)", k, name, x.Parent(), e.pos(x.Pos()))
	}
	return fmt.Sprintf("%T", k)
}

// explainSetup cannot know the source id before the first round creates it,
// so it runs one round, picks the id, and clears nothing: the record of why
// an object holds the tag starts from the second round, which is enough for
// a flow-insensitive fixpoint (every write is replayed each round).
func (e *engine) explainSetup(pos string) {
	e.run(40)
	for _, s := range e.sources {
		if strings.HasSuffix(s.Pos, pos) {
			e.explainID = s.ID
			e.why = map[objKey]string{}
			// Forget the objects tagged in round one so their first writer is
			// recorded again.
			for _, t := range e.objTags {
				if t.has(s.ID) {
					t.w[s.ID/64] &^= 1 << uint(s.ID%64)
				}
			}
			return
		}
	}
}
