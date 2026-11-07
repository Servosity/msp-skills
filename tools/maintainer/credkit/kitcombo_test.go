// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package credkit

// kitcombo (2026-11-06): kitcpu's constants table (consttable.go) is read
// through the matcher KF-JSM-1 changed (a Basic pair whose user is a
// declared Identifier holds only its pieces with a password character). A
// verdict read from a table must be the verdict the kit gives the same
// fixed text cold: for kitcpu's held sets, and for KF-JSM-1's shapes (an
// email user, an 8-space password, the "YY:     " unit of Codex round 4).

import (
	"encoding/base64"
	"math/rand/v2"
	"runtime"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
)

// kitcomboJSM is KF-JSM-1's units: the user alone, the user with its
// colon, pieces of the pair, as text and inside a constant statement.
var kitcomboJSM = []string{
	"tech@acmemsp.com", "tech@acmemsp.com:", `{"emailAddress":"tech@acmemsp.com"}`,
	"acmemsp.com", "https://acmemsp.atlassian.net/rest/servicedeskapi/servicedesk/1",
	"YY:     ", "yyyyyy:", "yyyyyy:        ", "jsmtok-Pr0beEm4il9z", "tech@acmemsp.com:jsmtok-Pr0beEm4il9z",
}

func kitcomboUnits() []string {
	u := kitcpuUnits()
	for _, s := range kitcomboJSM {
		u = append(u, s, kitcpuStatement(s))
	}
	return u
}

var kitcomboTable = sync.OnceValue(func() string {
	t, err := ConstantTable(kitcomboUnits(), false)
	if err != nil {
		panic(err)
	}
	return t
})

// kitcomboHeld is one held set: what it is, and how it is held.
type kitcomboHeld struct {
	name string
	hold func()
}

func holdEnv(vals ...string) func() {
	return func() { resetForTest(Declarations{}); Hold("env:K", vals...) }
}

func holdPair(d Declarations, user, pass string) func() {
	return func() {
		resetForTest(d)
		Hold("header:authorization", "Basic "+base64.StdEncoding.EncodeToString([]byte(user+":"+pass)))
	}
}

// kitcomboSets is kitcpu's held sets (kitcpu_test.go) and KF-JSM-1's
// (kitjsm_test.go), each pair both with its user declared and without.
func kitcomboSets(t *testing.T) []kitcomboHeld {
	var s []kitcomboHeld
	s = append(s, kitcomboHeld{"kitcpuCreds", holdEnv(kitcpuCreds...)}, kitcomboHeld{"kitcpuLeak", holdEnv(kitcpuLeak)})
	for _, v := range []string{"resource_type", "CURRENT_TIMESTAMP", "TEXT", "id", "12345678", "Größe-resources", "idx_learn_unique ON"} {
		s = append(s, kitcomboHeld{"sql " + v, holdEnv(v)})
	}
	for _, e := range publicText() {
		var full []rune
		for _, c := range e {
			full = append(full, c+0xFEE0)
		}
		for _, v := range []string{e, strings.ToUpper(e), rot13s(e), string(full)} {
			s = append(s, kitcomboHeld{"public " + v, holdEnv(v)})
		}
	}
	// The windows the closure decodes out of public text, as kitcpu's
	// TestKitcpuStartupVerdictUnchanged finds them.
	ref := newMatcher([]string{constRef})
	var mu sync.Mutex
	decoded := map[string]bool{}
	ref.rec = func(b []byte) {
		mu.Lock()
		defer mu.Unlock()
		run := 0
		for i := range b {
			if b[i] >= 'a' && b[i] <= 'z' || b[i] >= '0' && b[i] <= '9' {
				run++
			} else {
				run = 0
			}
			if run >= 10 {
				decoded[string(b[i+1-10:i+1])] = true
			}
		}
	}
	for _, u := range append(publicText(), kitcomboUnits()...) {
		ref.check([]byte(u))
	}
	var dec []string
	for v := range decoded {
		dec = append(dec, v)
	}
	slices.Sort(dec)
	if len(dec) == 0 {
		t.Fatal("no decoded window found")
	}
	for _, v := range dec {
		s = append(s, kitcomboHeld{"decoded " + v, holdEnv(v)})
	}
	// KF-JSM-1's pairs: its fixed cases and its generated ones (the same
	// generator and seed as TestKitJSMPositionalProperty).
	pairs := [][2]string{
		{"tech@acmemsp.com", "jsmtok-Pr0beEm4il9z"}, {"tech@acmemsp.com", strings.Repeat(" ", 8)},
		{"yyyyyy", strings.Repeat(" ", 8)}, {"yyyyyyyy", strings.Repeat(" ", 8)}, {"tech@acmemsp.com", "x"},
		{"tech@Ǻ̖ﬃab", "jsmtok-Pr0beEm4il9z"}, {"tech@acme.oﬃce", "́́́́́́́́"},
		{"zzzzzzzz@zz.zz", "zzzzzzzz"}, {"a", "yyyy    "},
	}
	r := rand.New(rand.NewPCG(1, 2))
	pick := func(alpha []string, lo, hi int) string {
		var b strings.Builder
		for range lo + r.IntN(hi-lo+1) {
			b.WriteString(alpha[r.IntN(len(alpha))])
		}
		return b.String()
	}
	ua := []string{"a", "e", "y", "y", "z", "t", "c", "h", "m", "s", "p", ".", "-"}
	pa := []string{" ", " ", "y", "Y", "z", "!", "0", "9", ":", "́", "̊", "ﬃ", "A", "k", "~"}
	n := 160
	if testing.Short() {
		n = 40
	}
	for range n {
		user := pick(ua, 1, 10)
		if r.IntN(2) == 0 {
			user += "@" + pick(ua, 1, 8) + "." + pick([]string{"com", "net", "yy", "example"}, 1, 1)
		}
		pairs = append(pairs, [2]string{user, pick(pa, 1, 18)})
	}
	for _, p := range pairs {
		s = append(s, kitcomboHeld{"basic-user " + p[0] + ":" + p[1], holdPair(jsmUser, p[0], p[1])},
			kitcomboHeld{"basic " + p[0] + ":" + p[1], holdPair(Declarations{}, p[0], p[1])})
	}
	return s
}

// TestKitcomboTableEqualsCold: for every held set, every fixed text a
// table calls clean (the startup check's public text, a constant
// statement) is clean when the kit checks it cold, and a pair held with
// its user cut reads the table through the cut matcher.
func TestKitcomboTableEqualsCold(t *testing.T) {
	consts.Lock()
	saved := slices.Clone(consts.tables)
	consts.Unlock()
	defer func() {
		consts.Lock()
		consts.tables = saved
		consts.Unlock()
	}()
	RegisterConstants(kitcomboTable())
	public := publicUnits()
	units := kitcomboUnits()
	type job struct {
		set  string
		m    *matcher // nil: the SQL path, withheld under set's state
		unit []byte
	}
	var (
		sets, cut, pubTable, pubCold, sqlTable, sqlCold atomic.Int64
		jobs                                            []job
	)
	run := func() {
		var bad atomic.Int64
		var next atomic.Int64
		var wg sync.WaitGroup
		for range runtime.GOMAXPROCS(0) {
			wg.Go(func() {
				for i := int(next.Add(1) - 1); i < len(jobs); i = int(next.Add(1) - 1) {
					j := jobs[i]
					var hit bool
					if j.m != nil {
						hit = j.m.check(j.unit) != clean
					} else {
						hit = withheld(sqlUnit(string(j.unit), nil))
					}
					if hit && bad.Add(1) <= 20 {
						t.Errorf("%s: the table called %q clean; cold, the kit withholds it", j.set, j.unit)
					}
				}
			})
		}
		wg.Wait()
		jobs = jobs[:0]
	}
	for _, h := range kitcomboSets(t) {
		h.hold()
		sets.Add(1)
		reg.Lock()
		vals := make([]string, 0, len(reg.vals))
		for v := range reg.vals {
			vals = append(vals, v)
		}
		slices.Sort(vals)
		cuts := userCutsLocked()
		reg.Unlock()
		if len(cuts) > 0 {
			cut.Add(1)
			// The table path's matcher is the cut one: its prefixes are
			// those of a fresh cut build, not of the pair held whole.
			m, _ := currentMatcher()
			fresh := buildMatcher(vals, cuts, false, false)
			if (m.prefixes == nil) != (fresh.prefixes == nil) || len(m.prefixes) != len(fresh.prefixes) {
				t.Errorf("%s: the table path's matcher is not the cut build", h.name)
			}
			for p := range m.prefixes {
				if _, ok := fresh.prefixes[p]; !ok {
					t.Errorf("%s: a prefix the cut build does not have", h.name)
					break
				}
			}
		}
		// Startup: public text, by the whole-value matcher it uses.
		if all := newWholeMatcher(vals); all != nil && all.constClean(true, public...) {
			pubTable.Add(1)
			cold := newWholeMatcher(vals) // fresh: no state shared with the one read
			for _, u := range public {
				jobs = append(jobs, job{h.name, cold, u})
				pubCold.Add(1)
			}
		}
		// SQL: each constant statement, by the live held set's matcher.
		var sqlJobs []job
		for _, u := range units {
			if constUnitClean([]byte(u)) {
				sqlTable.Add(1)
				sqlJobs = append(sqlJobs, job{h.name, nil, []byte(u)})
			}
		}
		if len(sqlJobs) > 0 { // withheld reads the live state: run before the next set
			jobs = append(jobs, sqlJobs...)
			sqlCold.Add(int64(len(sqlJobs)))
			run()
		}
		if len(jobs) > 4096 {
			run()
		}
	}
	run()
	t.Logf("held sets %d (cut pairs %d); startup table-clean %d sets, %d entries checked cold; SQL table-clean %d units, %d checked cold",
		sets.Load(), cut.Load(), pubTable.Load(), pubCold.Load(), sqlTable.Load(), sqlCold.Load())
	if pubTable.Load() == 0 || sqlTable.Load() == 0 || cut.Load() == 0 {
		t.Error("a path was never read from the table")
	}
}
