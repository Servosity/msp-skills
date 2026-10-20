// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"net/http"
	"strings"
	"testing"

	"eset-protect-pp-cli/internal/cliutil/credvariants"
	"eset-protect-pp-cli/internal/cliutil/testenv"
)

const downloadErrorSentinel = "DLERRSECRET-8P3V"

// TestQuarantineDownloadErrorsNeverShowADeviceToken (Codex round 5, P1): the
// two quarantine downloads declare a binary response, and client.do skipped
// the deviceToken redactor for their HTTP error bodies too. A proxy or ESET
// answering 400 with a JSON (or form-encoded) body naming a device token put
// it on stderr, and with --json into the error envelope on stdout. Every
// error body now goes through the redact-or-withhold funnel; only a
// successful download skips it.
func TestQuarantineDownloadErrorsNeverShowADeviceToken(t *testing.T) {
	testenv.Isolate(t)
	respondClearHarness(t)
	bodies := []struct{ contentType, body string }{
		{"application/json", `{"code":3,"message":"bad filter","details":[{"deviceToken":"` + downloadErrorSentinel + `"}]}`},
		{"application/octet-stream", `{"deviceToken":"` + downloadErrorSentinel + `"}`},
		{"text/plain", `deviceToken=` + downloadErrorSentinel},
		{"", `{"device\u0054oken":"` + downloadErrorSentinel + `"} trailing`},
	}
	current := 0
	respondFakeESET(t, func(w http.ResponseWriter, r *http.Request) {
		if ct := bodies[current].contentType; ct != "" {
			w.Header().Set("Content-Type", ct)
		}
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(bodies[current].body))
	})
	for i := range bodies {
		current = i
		for _, command := range []string{"quarantined-objects-download", "quarantined-objects-batch-download"} {
			for _, extra := range [][]string{nil, {"--json"}} {
				args := append([]string{command}, extra...)
				if command == "quarantined-objects-batch-download" {
					args = append(args, "--object-uuids", "00000000-0000-0000-0000-0000000000c1")
				}
				out, errOut, err := captureExecuteAll(t, args...)
				if err == nil {
					t.Fatalf("%v: an HTTP 400 must fail", args)
				}
				for name, b := range map[string][]byte{"stdout": out, "stderr": errOut, "error": []byte(err.Error())} {
					if credvariants.Recoverable(b, downloadErrorSentinel) {
						t.Fatalf("%v (body %q): %s carries the device token: %s", args, bodies[i].body, name, b)
					}
				}
				if !strings.Contains(err.Error(), "400") {
					t.Fatalf("%v: the error no longer names the status: %v", args, err)
				}
			}
		}
	}
}
