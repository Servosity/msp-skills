// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package credkit

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math/rand"
	"net/url"
	"strings"
	"testing"
	"time"

	"golang.org/x/text/encoding/charmap"
	"golang.org/x/text/encoding/japanese"
	"golang.org/x/text/encoding/simplifiedchinese"
)

// modalViews runs modal over p, the walk or (whole) each reading decoded
// whole as before, and returns the views it queued and whether it reported
// a document open where the unit ends.
func modalViews(p []byte, tail, whole bool) (map[string]bool, bool) {
	m := newMatcher([]string{"zT9kQ2vX7mLp4RwN8sYb3HcJ6fDg1AeUo5Ki0VtZ"})
	m.prepare()
	c := &closure{m: m}
	c.budget.Store(1 << 40)
	c.deflLeft.Store(1 << 40)
	c.done, c.seen = new(stripedSet), new(stripedSet)
	c.done.init(1024)
	c.seen.init(64)
	defer func(w bool) { modalWhole = w }(modalWhole)
	modalWhole = whole
	d := decoding{s: c.worker(0), depth: 1, tail: tail}
	d.modal(japanese.ISO2022JP, p, 0x1b, jpSwitch)
	d.modal(simplifiedchinese.HZGB2312, p, '~', hzSwitch)
	views := map[string]bool{}
	for f := range d.bk {
		for _, part := range [][]byte{d.bk[f].tany, d.bk[f].bjunk} {
			for _, pc := range bytes.Split(part, []byte{sep}) {
				if len(pc) > 0 {
					views[string(pc)] = true
				}
			}
		}
	}
	return views, c.open.Load()
}

// modalInput is a part with escapes of both modal charsets, documents,
// quotes, line feeds and high bytes at random.
func modalInput(r *rand.Rand, n int) []byte {
	pieces := []string{"~{", "~}", "~~", "~\n", "~", "\x1b$B", "\x1b(B", "\x1b(J", "\x1b(I", "\x1b$(D", "\x1b$@", "\x1b",
		`{"a":"b"}`, `["x",1]`, `{"k":`, `"v"`, "[", "{", "}", "]", `"`, "\n", " ", ",", ":", "\\u0041", "%41",
		"abcdef", "0123", "\x80", "\xa4\xa2", "\xc0", "\x00", "zT9kQ2vX"}
	var b []byte
	for len(b) < n {
		if r.Intn(3) == 0 {
			b = append(b, byte(r.Intn(256)))
			continue
		}
		b = append(b, pieces[r.Intn(len(pieces))]...)
	}
	return b
}

// The walk reads every view modal read decoding each reading whole, and
// reports an open document wherever it did.
func TestKitsafeModalWalkReadsWhatWholeReadingsRead(t *testing.T) {
	r := rand.New(rand.NewSource(495))
	inputs := [][]byte{
		[]byte(`[{"body":"strike ~~this~~ out","id":1},{"body":"~{ and ~} too","id":2}]`),
		[]byte("{\"a\":\"\x1b$B$3$s\x1b(B\",\n\"b\":[\"~{<:~}\"]}\n{\"c\":\"d\"}"),
	}
	for i := 0; i < 600; i++ {
		inputs = append(inputs, modalInput(r, 1+r.Intn(3000)))
	}
	for i, p := range inputs {
		for _, tail := range []bool{false, true} {
			old, oldOpen := modalViews(p, tail, true)
			got, gotOpen := modalViews(p, tail, false)
			for v := range old {
				if !got[v] {
					t.Fatalf("input %d (tail %v, %q): the walk lost view %q", i, tail, p, v)
				}
			}
			if oldOpen && !gotOpen {
				t.Fatalf("input %d (tail %v, %q): the walk lost the open document", i, tail, p)
			}
		}
	}
}

// tableViews runs tables over p, each joined reading built whole as before
// (whole) or parsed from each document's start, and returns the views it
// queued and whether it reported a document open where the unit ends.
func tableViews(p []byte, tail, whole bool) (map[string]bool, bool) {
	m := newMatcher([]string{"zT9kQ2vX7mLp4RwN8sYb3HcJ6fDg1AeUo5Ki0VtZ"})
	m.prepare()
	c := &closure{m: m}
	c.budget.Store(1 << 40)
	c.deflLeft.Store(1 << 40)
	c.done, c.seen = new(stripedSet), new(stripedSet)
	c.done.init(1024)
	c.seen.init(64)
	defer func(w bool) { tableWhole = w }(tableWhole)
	tableWhole = whole
	d := decoding{s: c.worker(0), depth: 1, tail: tail}
	d.tables(p)
	views := map[string]bool{}
	for f := range d.bk {
		for _, part := range [][]byte{d.bk[f].tany, d.bk[f].bjunk} {
			for _, pc := range bytes.Split(part, []byte{sep}) {
				if len(pc) > 0 {
					views[string(pc)] = true
				}
			}
		}
	}
	return views, c.open.Load()
}

// tableInput is a binary part with documents, quotes, brackets and line
// feeds of the EBCDIC code pages among random bytes.
func tableInput(r *rand.Rand, n int) []byte {
	var pieces [][]byte
	for _, e := range []*charmap.Charmap{charmap.CodePage037, charmap.CodePage1047, charmap.CodePage1140} {
		for _, s := range []string{`{"a":"b"}`, `["x",1]`, `{"k":`, `"v"`, "[", "{", "}", "]", `"`, "\n", " ", ",", ":",
			`{"a":["b",{"c":"d"}]}`, "abc", "\\u0041"} {
			b, err := e.NewEncoder().Bytes([]byte(s))
			if err == nil {
				pieces = append(pieces, b)
			}
		}
	}
	var b []byte
	for len(b) < n {
		if r.Intn(3) == 0 {
			b = append(b, byte(r.Intn(256)))
			continue
		}
		b = append(b, pieces[r.Intn(len(pieces))]...)
	}
	return b
}

// Parsing each document from its start reads every view the whole reading
// read, and reports an open document wherever it did.
func TestKitsafeTableDocsReadWhatWholeReadingsRead(t *testing.T) {
	r := rand.New(rand.NewSource(495))
	for i := 0; i < 600; i++ {
		p := tableInput(r, 1+r.Intn(3000))
		for _, tail := range []bool{false, true} {
			old, oldOpen := tableViews(p, tail, true)
			got, gotOpen := tableViews(p, tail, false)
			if len(old) == 0 && i == 0 {
				t.Fatalf("input %d read no view: the test reads nothing", i)
			}
			for v := range old {
				if !got[v] {
					t.Fatalf("input %d (tail %v, %q): the parse from each start lost view %q", i, tail, p, v)
				}
			}
			if oldOpen && !gotOpen {
				t.Fatalf("input %d (tail %v, %q): the parse from each start lost the open document", i, tail, p)
			}
		}
	}
}

// Field-found (issue #495, 2026-11-04): ordinary rich text spent the budget
// and was withheld whole. The fixtures are the shapes the field returned.

// kitsafeHeld is a stand-in held value, never a real one.
const kitsafeHeld = "zT9kQ2vX7mLp4RwN8sYb3HcJ6fDg1AeUo5Ki0VtZ"

// safeLinksEmails is n HTML email articles, each with eight Outlook
// SafeLinks in the real format: the url parameter percent-encoded, data a
// pipe list with hex and base64, sdata base64.
func safeLinksEmails(n int) []byte {
	var arts []any
	for i := 0; i < n; i++ {
		var b strings.Builder
		b.WriteString("<div>Please see the links below.<br>")
		for j := 0; j < 8; j++ {
			inner := fmt.Sprintf("https://portal.vendor.example/kb/article?id=%d&utm_source=email&utm_campaign=nov%d", 500+j, i)
			g := sha256.Sum256([]byte(fmt.Sprint(i, j)))
			data := fmt.Sprintf("05|02|pat.doe@contoso.example|%x|%x|0|0|6386%014d|Unknown|TWFpbGZsb3d8eyJWIjoiMC4wLjAwMDAiLCJQIjoiV2luMzIiLCJBTiI6Ik1haWwiLCJXVCI6Mn0=|0|||", g[:16], g[16:], i*100+j)
			sdata := base64.StdEncoding.EncodeToString(g[:])
			fmt.Fprintf(&b, `<a href="https://nam02.safelinks.protection.outlook.com/?url=%s&amp;data=%s&amp;sdata=%s&amp;reserved=0" originalsrc="%s">KB %d</a><br>`,
				url.QueryEscape(inner), url.QueryEscape(data), url.QueryEscape(sdata), inner, 500+j)
		}
		b.WriteString("</div>")
		arts = append(arts, map[string]any{"id": 90000 + i, "ticket_id": 4421, "type": "email", "content_type": "text/html", "body": b.String()})
	}
	out, _ := json.MarshalIndent(arts, "", "  ")
	return out
}

// incidentReports is a page of n Huntress-shaped incident reports
// (issue495Page in skills/huntress/cli/internal/client): HTML bodies with
// Windows paths, a PowerShell -EncodedCommand base64 blob and SHA256 hex.
func incidentReports(n int) []byte {
	var reports []any
	for i := 0; i < n; i++ {
		sum := sha256.Sum256([]byte(fmt.Sprint("file", i)))
		enc := base64.StdEncoding.EncodeToString([]byte(strings.Repeat(fmt.Sprintf("Get-Service -Name Spooler%d | Select-Object Status, StartType;", i), 6)))
		var b strings.Builder
		for j := 0; j < 12; j++ {
			fmt.Fprintf(&b, `<p>Process <code>C:\Windows\System32\WindowsPowerShell\v1.0\powershell.exe -EncodedCommand %s</code> spawned by <b>C:\Users\jdoe\AppData\Local\Temp\x%d.exe</b> (SHA256 <code>%s</code>) contacted <a href="https://host.example/p?q=a%%20b&amp;r=%d">host</a> &lt;host&gt;</p>`, enc[:120], j, hex.EncodeToString(sum[:]), j)
		}
		reports = append(reports, map[string]any{"id": 1000 + i, "status": "sent", "summary": "Suspicious script activity", "body": b.String()})
	}
	out, _ := json.Marshal(map[string]any{"incident_reports": reports, "pagination": map[string]any{"current_page": 1}})
	return out
}

// inlineImageEmails is n HTML email articles with a quoted thread, a
// signature and a small inline PNG data URI.
func inlineImageEmails(n int) []byte {
	png := base64.StdEncoding.EncodeToString([]byte(strings.Repeat("\x89PNG\r\n\x1a\nIHDR logo-bytes ", 120)))
	var arts []any
	for i := 0; i < n; i++ {
		body := fmt.Sprintf(`<div>Hi team,<br><br>The backup job on SRV-FS%02d failed again last night with error 0x80070005 (Access is denied). `+
			`Log path: C:\ProgramData\Agent\logs\job-%d.log. Can you check the service account?<br><br>`+
			`Thanks,<br><b>Pat Doe</b><br>IT Manager, Contoso Ltd<br><a href="https://contoso.example/support?ref=%d">contoso.example</a><br>`+
			`<img src="data:image/png;base64,%s" alt="logo"></div>`+
			`<blockquote type="cite"><div>On Mon, Nov 2, 2026 at 9:%02d AM Support &lt;support@msp.example&gt; wrote:<br>`+
			`We restarted the job and it completed. Ticket #%d will auto-close in 3 days.<br>Reference: %x</div></blockquote>`,
			i, 1000+i, i, png, i%60, 40000+i, []byte(fmt.Sprintf("sha256-of-%d-msg", i)))
		arts = append(arts, map[string]any{
			"id": 90000 + i, "ticket_id": 4200 + i/3, "type": "email", "from": "Pat Doe <pat.doe@contoso.example>",
			"subject": fmt.Sprintf("RE: Backup failed on SRV-FS%02d [Ticket#%d]", i, 40000+i), "body": body, "content_type": "text/html",
			"created_at": time.Date(2026, 11, 2, 9, i%60, 0, 0, time.UTC).Format(time.RFC3339),
		})
	}
	b, _ := json.MarshalIndent(arts, "", "  ")
	return b
}

// Ordinary rich text never spends the budget: every size reads clean.
func TestKitsafeOrdinaryUnitsCheckClean(t *testing.T) {
	m := newMatcher([]string{kitsafeHeld})
	cases := []struct {
		name  string
		build func(int) []byte
		sizes []int
	}{
		{"SafeLinks emails", safeLinksEmails, []int{1, 3, 10, 30, 100}},
		{"incident reports", incidentReports, []int{10, 25, 100}},
		{"inline-image emails", inlineImageEmails, []int{100}},
	}
	for _, c := range cases {
		for _, n := range c.sizes {
			unit := c.build(n)
			if (raceBuild || testing.Short()) && len(unit) > 64<<10 {
				t.Logf("%s x%d (%d bytes): skipped under -race and -short", c.name, n, len(unit))
				continue
			}
			if v := m.check(unit); v != clean {
				t.Errorf("%s x%d (%d bytes, no credential): verdict %d, want clean", c.name, n, len(unit), v)
			}
		}
	}
}

// The matcher's work on ordinary rich text grows at most linearly with its
// size: on n copies of one body, and per byte over more distinct bodies.
// Proved both ways: modal decoding each reading whole (the quadratic this
// lane removed) fails the per-byte bound.
func TestKitsafeOrdinaryWorkIsLinear(t *testing.T) {
	m := newMatcher([]string{kitsafeHeld})
	work := func(unit []byte) int64 {
		_, _, dec, defl := m.checkWork(unit)
		return dec + defl
	}
	for name, body := range map[string][]byte{"SafeLinks": safeLinksEmails(1), "incident": incidentReports(1), "inline image": inlineImageEmails(1)} {
		if name == "incident" {
			body = body[len(`{"incident_reports":`):bytes.Index(body, []byte(`,"pagination"`))]
		}
		inner := body[1 : len(body)-1]
		copies := func(n int) []byte {
			out := []byte{'['}
			for i := 0; i < n; i++ {
				if i > 0 {
					out = append(out, ',')
				}
				out = append(out, inner...)
			}
			return append(out, ']')
		}
		one := work(copies(1))
		for _, n := range []int{2, 4, 8} {
			if w := work(copies(n)); float64(w) > 1.1*float64(n)*float64(one) {
				t.Errorf("%s: %d copies did %d work, more than %d times one copy's %d", name, n, w, n, one)
			}
		}
	}
	perByte := func(n int) float64 {
		defer func(b, d int) { budgetPerByte, deflPerByte = b, d }(budgetPerByte, deflPerByte)
		budgetPerByte, deflPerByte = 1<<14, 1<<14 // measure past the budget
		unit := safeLinksEmails(n)
		_, _, dec, _ := m.checkWork(unit)
		return float64(dec) / float64(len(unit))
	}
	if a, b := perByte(4), perByte(16); b > 1.5*a {
		t.Errorf("SafeLinks emails: %.0f decoded bytes per byte at 16, %.0f at 4: superlinear", b, a)
	}
	defer func(w bool) { modalWhole = w }(modalWhole)
	modalWhole = true
	if a, b := perByte(4), perByte(16); b <= 1.5*a {
		t.Errorf("each modal reading decoded whole: %.0f per byte at 16, %.0f at 4: the bound does not catch the quadratic", b, a)
	}
}

// A held credential inside a SafeLinks email is still withheld: raw in the
// body, percent-encoded inside the url parameter, and base64 inside sdata.
func TestKitsafeHeldInSafeLinksIsWithheld(t *testing.T) {
	m := newMatcher([]string{kitsafeHeld})
	for _, n := range []int{1, 10} {
		page := string(safeLinksEmails(n))
		first := `sdata=` + url.QueryEscape(base64.StdEncoding.EncodeToString(func() []byte { g := sha256.Sum256([]byte(fmt.Sprint(0, 0))); return g[:] }()))
		if !strings.Contains(page, first) {
			t.Fatalf("fixture: no sdata %q", first)
		}
		for name, unit := range map[string]string{
			"raw":          strings.Replace(page, "Please see", "Please see "+kitsafeHeld, 1),
			"piece":        strings.Replace(page, "Please see", "Please see x"+kitsafeHeld[5:13]+"x", 1),
			"url percent":  strings.Replace(page, "url=https%3A%2F%2Fportal", "url=https%3A%2F%2Fportal.vendor.example%2F%3Ftoken%3D"+url.QueryEscape(url.QueryEscape(kitsafeHeld))+"%26x%3D", 1),
			"url param":    strings.Replace(page, "url=https%3A%2F%2Fportal", "url="+url.QueryEscape("https://x.example/?k="+kitsafeHeld+"&r=")+"portal", 1),
			"sdata base64": strings.Replace(page, first, "sdata="+url.QueryEscape(base64.StdEncoding.EncodeToString([]byte("sig:"+kitsafeHeld))), 1),
		} {
			if unit == page {
				t.Fatalf("%s: fixture unchanged", name)
			}
			if v := m.check([]byte(unit)); v != leak {
				t.Errorf("%s, %d emails: verdict %d, want leak (withheld)", name, n, v)
			}
		}
	}
}

// A unit can end anywhere: the stream splits a write where it pauses
// (stdio.go, unitIdle), and a pump read is at most 64 KiB. A unit cut inside
// rich text reads nested fixed-Huffman repeats a whole document does not, a
// cost of its own that does not grow with the unit (limits.go, the bases).
// These are the costliest of 674 cuts measured on a 3 KiB grid.
func TestKitsafeCutUnitsCheckClean(t *testing.T) {
	if raceBuild || testing.Short() {
		t.Skip("each cut takes seconds; skipped under -race and -short")
	}
	m := newMatcher([]string{kitsafeHeld})
	b := safeLinksEmails(12)
	for _, c := range [][2]int{{12288, 33792}, {12288, 30720}, {0, 39936}, {0, 64512}} {
		if v := m.check(b[c[0]:c[1]]); v != clean {
			t.Errorf("SafeLinks emails cut to [%d:%d] (no credential): verdict %d, want clean", c[0], c[1], v)
		}
	}
}
