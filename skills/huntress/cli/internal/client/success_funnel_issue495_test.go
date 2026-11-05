// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
//
// Hand-written (NOT generated): issue #495 regression tests for the success-body
// check. Pinned by handfixes.json under "secret-echo-decoded-funnel".

package client

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

// issue495Page is a page of n incident reports shaped like ordinary Huntress
// report bodies (HTML, Windows paths, a base64-encoded script argument, file
// hashes, links), with inject appended to the last report's body.
func issue495Page(n int, inject string) []byte {
	var reports []any
	for i := 0; i < n; i++ {
		sum := sha256.Sum256([]byte(fmt.Sprint("file", i)))
		enc := base64.StdEncoding.EncodeToString([]byte(strings.Repeat(fmt.Sprintf("Get-Service -Name Spooler%d | Select-Object Status, StartType;", i), 6)))
		var b strings.Builder
		for j := 0; j < 12; j++ {
			fmt.Fprintf(&b, `<p>Process <code>C:\Windows\System32\WindowsPowerShell\v1.0\powershell.exe -EncodedCommand %s</code> spawned by <b>C:\Users\jdoe\AppData\Local\Temp\x%d.exe</b> (SHA256 <code>%s</code>) contacted <a href="https://host.example/p?q=a%%20b&amp;r=%d">host</a> &lt;host&gt;</p>`, enc[:120], j, hex.EncodeToString(sum[:]), j)
		}
		if i == n-1 {
			b.WriteString(inject)
		}
		reports = append(reports, map[string]any{"id": 1000 + i, "status": "sent", "summary": "Suspicious script activity", "body": b.String()})
	}
	out, _ := json.Marshal(map[string]any{"incident_reports": reports, "pagination": map[string]any{"current_page": 1}})
	return out
}

// Issue #495: on 0.1.11 a page of five or more ordinary incident reports spent
// the funnel's work budget and was refused as an echo. Every page size up to
// Huntress's largest is returned.
func TestSuccessFunnelIssue495OrdinaryPageIsReturned(t *testing.T) {
	for _, n := range []int{1, 5, 25, 100} {
		page := issue495Page(n, "")
		srv, _ := newClassFunnelServer(t, "application/json", page)
		c := newClassFunnelClient(t, srv, classFunnelSecret16, false)
		got, err := c.Get(context.Background(), "/v1/incident_reports", nil)
		if err != nil {
			t.Fatalf("a page of %d ordinary incident reports (%d bytes) was refused: %v", n, len(page), err)
		}
		if len(got) == 0 {
			t.Fatalf("a page of %d reports returned nothing", n)
		}
	}
}

// Guard for the #495 fix: checking a page in windows cut between tokens still
// refuses an echo, whole or as an 8-character piece, plain or encoded, in a
// large page.
func TestSuccessFunnelIssue495EchoInLargePageIsRefused(t *testing.T) {
	for name, inject := range map[string]string{
		"plain":         " token " + classFunnelSecret16,
		"piece":         " x" + classFunnelSecret16[3:11] + "x",
		"base64":        " " + base64.StdEncoding.EncodeToString([]byte("pre "+classFunnelSecret16)),
		"hex":           " " + hex.EncodeToString([]byte(classFunnelSecret16)),
		"json-escaped":  ` \u0061\u0062\u0063\u0064` + classFunnelSecret16[4:],
		"percent":       " q=%61%62%63%64" + classFunnelSecret16[4:],
		"html-entities": " &#97;&#98;&#99;&#100;" + classFunnelSecret16[4:],
	} {
		for _, n := range []int{1, 25} {
			srv, _ := newClassFunnelServer(t, "application/json", issue495Page(n, inject))
			c := newClassFunnelClient(t, srv, classFunnelSecret16, false)
			if got, err := c.Get(context.Background(), "/v1/incident_reports", nil); err == nil {
				t.Fatalf("%s echo in a page of %d reports was returned (%d bytes)", name, n, len(got))
			}
		}
	}
}
