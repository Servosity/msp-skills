// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package credkit

import (
	"encoding/base64"
	"fmt"
	"math/rand/v2"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// The ordinary-output corpus: every rule the kit adds, a blunt fail-closed
// rule included, must leave real-shaped connector output clean (contract 3,
// Budget quality target). The corpus is the testdata and golden files under
// skills/*/cli (when the kit runs in the monorepo) plus units of the shapes
// connectors print: JSON with numeric ID arrays and timestamps, a base64
// attachment, an HTML description, log lines.
func ordinaryCorpus(t *testing.T) map[string][]byte {
	c := map[string][]byte{}
	for _, pat := range []string{"../../../skills/*/cli/*/*/testdata/*", "../../../skills/*/cli/*/*/*/testdata/*",
		"../../../skills/*/cli/*/testdata/*", "../../../skills/*/cli/*/*.golden", "../../../skills/*/cli/*/*/*.golden"} {
		files, _ := filepath.Glob(pat)
		for _, f := range files {
			if b, err := os.ReadFile(f); err == nil && len(b) > 0 {
				c[f] = b
			}
		}
	}
	r := rand.New(rand.NewPCG(5, 6))
	var ids []string
	for range 400 {
		ids = append(ids, fmt.Sprint(100000+r.IntN(900000)))
	}
	c["numeric ID array"] = []byte(`{"device_ids":[` + strings.Join(ids, ",") + `],"total":400,"page":1}`)
	att := make([]byte, 6000)
	for i := range att {
		att[i] = byte(r.IntN(256))
	}
	c["base64 attachment"] = []byte(fmt.Sprintf(`{"attachments":[{"id":9917,"filename":"screenshot.png",`+
		`"content_type":"image/png","size":%d,"data":%q}]}`, len(att), base64.StdEncoding.EncodeToString(att)))
	c["HTML description"] = []byte(`{"id":4411,"title":"Printer offline","description":"<div><p>Hi team,</p>` +
		`<p>The <b>2nd floor</b> printer (HP LaserJet M404n, S/N VNB3R12345) shows &quot;offline&quot; since 09:14.</p>` +
		`<ul><li>Restarted spooler</li><li>Pinged 10.0.4.23: 0% loss</li></ul>` +
		`<p>See <a href=\"https://helpdesk.example.com/kb/2231?ref=ticket&amp;lang=en\">KB 2231</a>.</p>` +
		`<p>Thanks,<br>Jane Doe &lt;jane@example.com&gt;</p></div>","created_at":"2026-10-14T09:21:07Z"}`)
	var logs strings.Builder
	for i := range 120 {
		fmt.Fprintf(&logs, "2026-10-%02dT%02d:%02d:%02d.%03dZ INFO agent[%d]: backup job %d %s in %dms (%d files, %d bytes) host=srv-%02d.corp.example.com\n",
			1+i%28, i%24, i%60, (i*7)%60, (i*37)%1000, 3000+i, 70000+i*13,
			[]string{"completed", "started", "queued", "retrying"}[i%4], 100+i*91, i*17, i*1048576, i%40)
	}
	c["log lines"] = []byte(logs.String())
	c["150 KB JSON"] = benchJSON(150 << 10)
	c["7 KB environment"] = benchEnv()
	c["Windows runner environment"] = windowsEnv()
	return c
}

func TestOrdinaryCorpusIsClean(t *testing.T) {
	corpus := ordinaryCorpus(t)
	if _, err := os.Stat("../../../skills"); err == nil && len(corpus) < 40 {
		t.Fatalf("the monorepo's testdata and golden files were not found: %d units", len(corpus))
	}
	// One worker as well as the machine's: the closure a unit spends
	// depends on the order its decoders run (kit11rv: one worker spent 110%
	// of the raw deflate budget on the base64 attachment, four spent 86%,
	// and a 4-CPU Windows runner exhausted on its own environment).
	for _, procs := range []int{runtime.GOMAXPROCS(0), 1} {
		prev := runtime.GOMAXPROCS(procs)
		for _, extra := range [][]string{nil, {nonASCIICred}} {
			resetForTest(Declarations{})
			Hold("bench", append(benchCreds(), extra...)...)
			m, _ := currentMatcher()
			for name, unit := range corpus {
				if v := m.check(unit); v != clean {
					t.Errorf("%s (non-ASCII credential held: %v, %d workers): verdict %d, want clean", name, extra != nil, procs, v)
				}
			}
		}
		runtime.GOMAXPROCS(prev)
	}
}
