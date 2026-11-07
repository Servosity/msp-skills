// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"io"
	"net/http"
	"strings"
	"testing"

	"eset-protect-pp-cli/internal/cliutil/testenv"
)

// TestEveryOutputModeKeepsBigIntegers (handfixes.json: json-numbers-exact) is
// the oracle for the whole number class: 2^53+1 at the top level, nested in an
// object and inside an array must come out digit for digit from a list and a
// get command in every output mode. --compact (also implied by --agent)
// rounded it through float64.
func TestEveryOutputModeKeepsBigIntegers(t *testing.T) {
	testenv.Isolate(t)
	respondClearHarness(t)
	const id = "00000000-0000-0000-0000-000000000001"
	device := `{"uuid":"` + id + `","displayName":"PC","count":` + bigInt + `,"meta":{"count":` + bigInt + `},"ranges":[{"uuid":"r1","count":` + bigInt + `}]}`
	respondFakeESET(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/v1/devices/"+id {
			_, _ = io.WriteString(w, `{"device":`+device+`}`)
			return
		}
		_, _ = io.WriteString(w, `{"devices":[`+device+`],"nextPageToken":""}`)
	})
	const rounded = "9007199254740992"
	for _, base := range [][]string{{"devices", "list"}, {"devices", "get", id}} {
		for _, mode := range [][]string{nil, {"--json"}, {"--compact"}, {"--agent"}, {"--select", "uuid,count,meta.count,ranges.count"}, {"--csv"}, {"--plain"}} {
			args := append(append([]string{}, base...), mode...)
			args = append(args, "--data-source", "live")
			out, errOut, err := captureExecuteAll(t, args...)
			if err != nil {
				t.Errorf("%v: %v\n%s", args, err, errOut)
				continue
			}
			// Compaction may drop a field by design; it may never round one.
			got := string(out)
			if strings.Contains(got, rounded) || (strings.Contains(got, "count") && !strings.Contains(got, bigInt)) {
				t.Errorf("%v rounded 2^53+1:\n%s", args, got)
			}
			if !strings.Contains(got, bigInt) && (len(mode) == 0 || mode[0] == "--json" || mode[0] == "--select" || mode[0] == "--compact" || mode[0] == "--agent") {
				t.Errorf("%v dropped the count field:\n%s", args, got)
			}
		}
	}
}
