package cli

import "testing"

func TestParseWindowDays(t *testing.T) {
	ok := []struct {
		in   string
		want float64
	}{
		{"30d", 30},
		{"12h", 0.5},
		{"24h", 1},
		{"2w", 14},
		{"7", 7},
		{"  3d ", 3},
		{"all", 0},
		{"0", 0},
	}
	for _, c := range ok {
		if got, err := parseWindowDays("since", c.in); err != nil || got != c.want {
			t.Errorf("parseWindowDays(%q) = %v, %v; want %v", c.in, got, err, c.want)
		}
	}
	// Each of these used to be read as something else (30m as 30 days, 1y
	// as 1 day, garbage as "no window"); a window must be refused instead.
	for _, in := range []string{"", "garbage", "-5d", "30m", "1y", "5x", "inf", "nan", "1e400d", "0d"} {
		if got, err := parseWindowDays("since", in); err == nil {
			t.Errorf("parseWindowDays(%q) = %v, want an error", in, got)
		}
	}
}

func TestCsvList(t *testing.T) {
	cases := []struct {
		in   string
		want []string
	}{
		{"", nil},
		{"a", []string{"a"}},
		{"a,b,c", []string{"a", "b", "c"}},
		{" a , b ,, c ", []string{"a", "b", "c"}},
	}
	for _, c := range cases {
		got := csvList(c.in)
		if len(got) != len(c.want) {
			t.Errorf("csvList(%q) len = %d, want %d (%v)", c.in, len(got), len(c.want), got)
			continue
		}
		for i := range got {
			if got[i] != c.want[i] {
				t.Errorf("csvList(%q)[%d] = %q, want %q", c.in, i, got[i], c.want[i])
			}
		}
	}
}

func TestInClause(t *testing.T) {
	if frag, args := inClause("col", nil); frag != "" || args != nil {
		t.Errorf("inClause empty = (%q,%v), want empty", frag, args)
	}
	frag, args := inClause("i.severity", []string{"critical", "high"})
	if frag != "i.severity IN (?,?)" {
		t.Errorf("inClause frag = %q", frag)
	}
	if len(args) != 2 || args[0] != "critical" || args[1] != "high" {
		t.Errorf("inClause args = %v", args)
	}
}

func TestToFloat(t *testing.T) {
	if f, ok := toFloat(int64(5)); !ok || f != 5 {
		t.Errorf("toFloat(int64 5) = %v,%v", f, ok)
	}
	if f, ok := toFloat("3.5"); !ok || f != 3.5 {
		t.Errorf("toFloat(\"3.5\") = %v,%v", f, ok)
	}
	if f, ok := toFloat([]byte("10")); !ok || f != 10 {
		t.Errorf("toFloat([]byte 10) = %v,%v", f, ok)
	}
	if _, ok := toFloat(nil); ok {
		t.Errorf("toFloat(nil) should be !ok")
	}
}

func TestJX(t *testing.T) {
	got := jx("i", "sent_at")
	want := "json_extract(i.data,'$.sent_at')"
	if got != want {
		t.Errorf("jx() = %q, want %q", got, want)
	}
}

func TestJDX(t *testing.T) {
	got := jdx("i", "sent_at")
	want := "julianday(replace(replace(json_extract(i.data,'$.sent_at'),'T',' '),'Z',''))"
	if got != want {
		t.Errorf("jdx() = %q, want %q", got, want)
	}
}
