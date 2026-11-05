// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package crowdstrikeblackbox

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Class credential-persisted-in-files, the jobs ledger: the donor connector
// recorded each asynchronous submit's job in state/jobs.jsonl, and its job id
// repeated the bearer token. The CrowdStrike CLI has no asynchronous submit
// and no `jobs` command, so the ledger scan has nothing to read; this keeps
// the property armed instead: after writes that echo the bearer token (host
// containment, a tag) and a fleet sync, any jobs ledger the program wrote is
// scanned like every other file, and the day a reprint adds a `jobs` command
// this test fails until a submit that records a job is added to the battery.
func TestJobLedgerHoldsNoCredential(t *testing.T) {
	t.Parallel()
	for _, mode := range []credMode{credStored, credPair, credPairCached} {
		t.Run(mode.String(), func(t *testing.T) {
			sv := echoServer(t)
			e := newEnv(t, sv, mode)
			out := e.runIn(t, `{"ids":["`+bbHost+`"]}`, "devices", "perform-action-v2", "--action-name", "contain", "--stdin", "--json")
			out += e.runIn(t, `{"action":"add","device_ids":["`+bbHost+`"],"tags":["FalconGroupingTags/HQ"]}`, "devices", "update-tags", "--stdin", "--json")
			out += e.run(t, "fleet", "sync", "--cids", bbChild, "--kinds", "hosts", "--json")
			methods, _, _, _ := sv.counts()
			var submitted bool
			for _, m := range methods {
				submitted = submitted || m == "POST /devices/entities/devices-actions/v2"
			}
			if !submitted {
				t.Fatalf("%s: the write never reached the fake (requests %v):\n%.1500s", mode, methods, out)
			}
			assertNoEcho(t, "write output", out)
			if jobs := e.run(t, "jobs", "list", "--json"); !strings.Contains(jobs, `unknown command "jobs"`) {
				t.Fatalf("%s: this CLI now has a jobs command; add a submit that records a job to this test:\n%.1500s", mode, jobs)
			}
			_ = filepath.WalkDir(e.home, func(path string, d fs.DirEntry, err error) error {
				if err != nil || d.IsDir() || !strings.HasPrefix(d.Name(), "jobs") {
					return nil
				}
				data, rerr := os.ReadFile(path)
				if rerr != nil {
					t.Fatal(rerr)
				}
				rel, _ := filepath.Rel(e.home, path)
				assertNoEcho(t, "file "+rel, string(data))
				return nil
			})
			checkMode(t, sv, mode)
		})
	}
}
