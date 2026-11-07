// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

// The live fan-out novels (fleet events, ip-conflicts, speedtest, triggers,
// unalerted)
// answer for every Collector the API key can see.

import (
	"fmt"
	"strconv"
	"strings"
	"testing"

	"domotz-pp-cli/internal/cliutil/testenv"
)

var liveFanoutNovels = []struct {
	args []string
	path string // per-agent path, %s = agent id
}{
	{[]string{"fleet", "events"}, "/agent/%s/activity-log"},
	{[]string{"fleet", "ip-conflicts"}, "/agent/%s/ip-conflict"},
	{[]string{"fleet", "speedtest"}, "/agent/%s/history/network/speed"},
	{[]string{"fleet", "triggers"}, "/agent/%s/device/eye/tcp"},
	{[]string{"fleet", "unalerted"}, "/alert-profile/binding/agent/%s/device"},
}

// domotz handfix live-fanout-walks-every-collector: GET /agent pages with
// page_size (default 10) and page_number (spec.json), and the live novels
// read one unpaged page, so an MSP with 23 Collectors got answers for 10.
func TestLiveNovelsWalkEveryCollector(t *testing.T) {
	testenv.Isolate(t)
	devices := map[string]string{}
	for i := 1; i <= 23; i++ {
		devices[strconv.Itoa(1000+i)] = `[]`
	}
	fake := newFakeDomotz(t, devices)
	for _, n := range liveFanoutNovels {
		before := len(fake.paths())
		runDomotz(t, append(n.args, "--json", "--no-learn")...)
		asked := map[string]bool{}
		for _, p := range fake.paths()[before:] {
			asked[p] = true
		}
		missing := 0
		for id := range devices {
			if !asked[fmt.Sprintf(n.path, id)] {
				missing++
			}
		}
		if missing > 0 {
			t.Errorf("%v asked %d of 23 Collectors", n.args, 23-missing)
		}
	}
}

// domotz handfix live-fanout-partial-is-not-whole: a Collector whose
// request failed was a stderr warning and the command exited 0 with the
// other Collectors' rows, so `fleet ip-conflicts --json` answered "no
// conflicts" for a site it never read. Any failed Collector fails the
// command (the rows it did read still print).
func TestLiveNovelFailedCollectorFailsTheCommand(t *testing.T) {
	testenv.Isolate(t)
	fake := newFakeDomotz(t, map[string]string{"100": `[]`, "200": `[]`})
	for _, n := range liveFanoutNovels {
		fake.change(func(_ map[string]string, _ map[string]bool) {})
		fake.mu.Lock()
		fake.failPaths = map[string]bool{fmt.Sprintf(n.path, "200"): true}
		fake.mu.Unlock()
		out, err := runDomotzErr(append(n.args, "--json", "--no-learn")...)
		if err == nil {
			t.Errorf("%v exited 0 with Collector 200 unread:\n%s", n.args, out)
		} else if !strings.Contains(err.Error(), "200") && !strings.Contains(out, "Site 200") {
			t.Errorf("%v failed without naming the Collector it could not read: %v", n.args, err)
		}
	}
}
