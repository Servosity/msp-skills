// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

package appdirectblackbox

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Class credential-persisted-in-files, the jobs ledger: an asynchronous submit
// (integration resource-other-bill-usage-v2-post) records the job the API
// answered in state/jobs.jsonl. The fake's job id repeats the first value the
// request carried, the bearer token, as its record ids do elsewhere; the
// ledger must not keep it (nor any held value), and jobs list must not print
// it, in either client-pair mode and with a stored token.
func TestJobLedgerHoldsNoCredential(t *testing.T) {
	for _, mode := range []credMode{credStored, credPair, credPairCached} {
		t.Run(mode.String(), func(t *testing.T) {
			sv := echoServer(t)
			e := newEnv(t, sv, mode)
			out := e.run(t, "integration", "resource-other-bill-usage-v2-post", "--billable", "--idempotency-key", "usage-1042", "--json")
			methods, _, _, _ := sv.counts()
			var submitted bool
			for _, m := range methods {
				submitted = submitted || m == "POST /api/integration/v2/billing/usage"
			}
			if !submitted {
				t.Fatalf("%s: the submit never reached the fake (requests %v):\n%.1500s", mode, methods, out)
			}
			assertNoEcho(t, "submit output", out)
			assertNoEcho(t, "jobs list output", e.run(t, "jobs", "list", "--json"))
			var ledgers int
			_ = filepath.WalkDir(e.home, func(path string, d fs.DirEntry, err error) error {
				if err != nil || d.IsDir() || d.Name() != "jobs.jsonl" {
					return nil
				}
				ledgers++
				data, rerr := os.ReadFile(path)
				if rerr != nil {
					t.Fatal(rerr)
				}
				if !strings.Contains(string(data), "resource-other-bill-usage-v2-post") {
					t.Errorf("%s: the ledger lost the job row itself:\n%s", mode, data)
				}
				rel, _ := filepath.Rel(e.home, path)
				assertNoEcho(t, "file "+rel, string(data))
				return nil
			})
			if ledgers == 0 {
				t.Fatalf("%s: the submit recorded no job; the test did not run:\n%.1500s", mode, out)
			}
			checkMode(t, sv, mode)
		})
	}
}
