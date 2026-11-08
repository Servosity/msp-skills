package credkit

import (
	"math/rand"
	"strings"
	"testing"
)

// The word set (#612) finds what the automaton alone finds: every held
// value, long and short, read in random texts that carry its pieces in
// random case, beside texts that carry none.
func TestKitmemWordsMatchAutomaton(t *testing.T) {
	r := rand.New(rand.NewSource(612))
	alpha := "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789-_.=%+/\x00é€"
	pick := func(n int) string {
		var b strings.Builder
		for b.Len() < n {
			s := []rune(alpha)
			b.WriteRune(s[r.Intn(len(s))])
		}
		return b.String()
	}
	build := func(vals []string, anywhere, whole bool, words bool) *matcher {
		defer func(o bool) { useWords = o }(useWords)
		useWords = words
		return buildMatcher(vals, nil, anywhere, whole)
	}
	flip := func(s string) string {
		b := []byte(s)
		for i, c := range b {
			if r.Intn(2) == 0 && c >= 'a' && c <= 'z' {
				b[i] = c - 'a' + 'A'
			} else if r.Intn(2) == 0 && c >= 'A' && c <= 'Z' {
				b[i] = c - 'A' + 'a'
			}
		}
		return string(b)
	}
	hits, n := 0, 0
	for round := 0; round < 300; round++ {
		var vals []string
		for k := r.Intn(6) + 1; k > 0; k-- {
			vals = append(vals, pick([]int{3, 7, 8, 9, 16, 40, 130}[r.Intn(7)]))
		}
		for _, mode := range [][2]bool{{false, false}, {true, false}, {false, true}} {
			a, w := build(vals, mode[0], mode[1], false), build(vals, mode[0], mode[1], true)
			if (a == nil) != (w == nil) {
				t.Fatalf("%q: one matcher is nil", vals)
			}
			if a == nil {
				continue
			}
			for k := 0; k < 20; k++ {
				text := pick(r.Intn(60))
				if k%2 == 0 {
					v := vals[r.Intn(len(vals))]
					i := r.Intn(len(v))
					piece := flip(v[i:min(len(v), i+r.Intn(len(v))+1)])
					switch k % 3 {
					case 0:
						text = piece // the whole text: a hit at its first and last byte
					case 1:
						text = piece + " " + text
					default:
						text += " " + piece + " " + pick(r.Intn(20))
					}
				}
				nt := normalize([]byte(text))
				for _, any := range []bool{false, true} {
					ga, gw := a.hitIn(nt, any), w.hitIn(nt, any)
					n++
					if ga {
						hits++
					}
					if ga != gw {
						t.Fatalf("held %q, mode %v, anywhere %v, text %q: automaton %v, words %v", vals, mode, any, text, ga, gw)
					}
				}
			}
		}
	}
	if hits == 0 || hits == n {
		t.Fatalf("%d of %d readings hit: the test reads nothing", hits, n)
	}
	t.Logf("%d of %d readings hit, the same in both", hits, n)
}
