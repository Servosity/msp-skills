// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
//
// Hand-written (NOT generated): pinned by skills/dnsfilter/handfixes.json
// under the "mcp-failure-keeps-stdout-remedy" entry.

package cobratree

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"dnsfilter-pp-cli/internal/cliutil"
	"dnsfilter-pp-cli/internal/mcp/redact"
)

// cutSecret is a site secret_key built from letters that never occur in the
// surrounding JSON or prose, so any 6-character run of it in an output is a
// leak (a cut can leave a suffix of the value).
func cutSecret(i int) string {
	const alpha = "QXZJVKWQZXJV"
	var b strings.Builder
	b.WriteString("SKLEAK")
	for n := uint64(i) + 7; b.Len() < 30; n = n*31 + 17 {
		b.WriteByte(alpha[n%uint64(len(alpha))])
	}
	return b.String()
}

func leaksAnySecret(out string, secrets []string) string {
	for _, s := range secrets {
		for i := 0; i+6 <= len(s); i++ {
			if strings.Contains(out, s[i:i+6]) {
				return s
			}
		}
	}
	return ""
}

// networkRow is one NDJSON tail row (event + a networks record) whose filler
// field has the given length.
func networkRow(i, filler int) string {
	return fmt.Sprintf(`{"event":"data","data":{"id":%d,"name":"Site %02d","physical_address":"%s","secret_key":"%s","organization_id":3}}`, i, i, strings.Repeat("a", filler), cutSecret(i))
}

// failureText runs the failed-command error path exactly as RunCLICommand
// does, then the MCP result funnel's Final, and returns what the client sees.
func failureText(stdout, stderr string) string {
	return redact.Final(cliFailureError("dnsfilter-cli", errors.New("exit status 2"), stdout, stderr).Error())
}

// TestFailureDetailFrontCutInsideKeyName: 25 rows, the 20-line tail cut from
// the front to failedStdoutDetailBytes, with the cut placed inside the first
// tail row's "secret_key" key name (the round 6 receipt shape).
func TestFailureDetailFrontCutInsideKeyName(t *testing.T) {
	t.Setenv(cliutil.MCPReadOnlyEnvVar, "1")
	// Rows are 403 bytes like the receipt; only the last row's filler varies,
	// which moves the front cut one byte at a time.
	for filler := 0; filler < 2000; filler++ {
		rows := make([]string, 25)
		secrets := make([]string, 25)
		for i := range rows {
			rows[i] = networkRow(i, 403-len(networkRow(i, 0)))
			secrets[i] = cutSecret(i)
		}
		rows[24] = networkRow(24, filler)
		tail := strings.Join(rows[5:], "\n")
		cut := len(tail) - failedStdoutDetailBytes
		keyAt := strings.Index(rows[5], `"secret_key"`)
		if cut != keyAt+4 {
			continue
		}
		out := failureText(strings.Join(rows, "\n")+"\n", "panic: interval must be positive")
		if s := leaksAnySecret(out, secrets); s != "" {
			t.Fatalf("filler %d: a front cut inside the key name leaked %s: %.600s", filler, s, out)
		}
		if !strings.Contains(out, "Site 24") {
			t.Fatalf("filler %d: the redacted rows were lost: %.300s", filler, out)
		}
		return
	}
	t.Fatal("no filler length put the cut inside the key name; fixture is wrong")
}

// TestFailureDetailLineTailBetweenKeyAndValue: the 20-line tail starts on the
// value line of a pretty-printed record whose key is on the line before.
func TestFailureDetailLineTailBetweenKeyAndValue(t *testing.T) {
	t.Setenv(cliutil.MCPReadOnlyEnvVar, "1")
	secret := cutSecret(99)
	var lines []string
	for i := 0; i < 5; i++ {
		lines = append(lines, fmt.Sprintf(`{"event":"progress","n":%d}`, i))
	}
	lines = append(lines, "{", `  "name": "Site 99",`, `  "secret_key":`)
	// The tail keeps the last 20 lines: put the value line exactly 20th from the end.
	value := `    "` + secret + `"`
	rest := []string{value, "}"}
	for i := 0; len(rest) < 20; i++ {
		rest = append(rest, fmt.Sprintf(`{"event":"progress","n":%d}`, 100+i))
	}
	stdout := strings.Join(append(lines, rest...), "\n")
	out := failureText(stdout, "Error: sync failed")
	if s := leaksAnySecret(out, []string{secret}); s != "" {
		t.Fatalf("a line tail between key and value leaked the secret: %.600s", out)
	}
}

// TestFailureDetailCaptureCapInsideValue: a stdout cut by the capture cap in
// the middle of a row's secret value keeps the other rows instead of
// withholding everything, and never shows the value.
func TestFailureDetailCaptureCapInsideValue(t *testing.T) {
	t.Setenv(cliutil.MCPReadOnlyEnvVar, "1")
	var rows []string
	var secrets []string
	for i := 0; i < 10; i++ {
		rows = append(rows, networkRow(i, 50))
		secrets = append(secrets, cutSecret(i))
	}
	full := strings.Join(rows, "\n")
	last := rows[len(rows)-1]
	capAt := len(full) - len(last) + strings.Index(last, cutSecret(9)) + 10
	out := failureText(full[:capAt], "Error: sync failed")
	if s := leaksAnySecret(out, secrets); s != "" {
		t.Fatalf("a capture cut inside a value leaked %s: %.600s", s, out)
	}
	if !strings.Contains(out, "Site 08") {
		t.Fatalf("the complete rows were withheld with the cut one: %.400s", out)
	}
}

// TestStderrHintSelectionAfterRedaction: a hint line is selected only from
// stderr that was redacted whole, so a value whose credential header sits on
// the line before the hint never reaches the hints block.
func TestStderrHintSelectionAfterRedaction(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell helper")
	}
	t.Setenv(cliutil.MCPReadOnlyEnvVar, "1")
	secret := cutSecret(777)
	dir := t.TempDir()
	errFile := filepath.Join(dir, "stderr.txt")
	// A table whose header names a credential column: the hint-looking row
	// carries the secret, and only the whole text shows it is a credential.
	if err := os.WriteFile(errFile, []byte("id,client_secret\nhint: ,"+secret+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	bin := filepath.Join(dir, "helper.sh")
	if err := os.WriteFile(bin, []byte("#!/bin/sh\necho '{\"ok\":true}'\ncat '"+errFile+"' >&2\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	res, err := RunCLICommand(context.Background(), bin, nil)
	if err != nil {
		t.Fatal(err)
	}
	// Checked on the selected hints themselves (not through Final, whose
	// withheld-value memory could mask the selection order).
	if s := leaksAnySecret(strings.Join(res.StderrHints, "\n"), []string{secret}); s != "" {
		t.Fatalf("a stderr hint selected before redaction leaked the secret: %q", res.StderrHints)
	}
}

// TestFailureDetailCutSweep is the property sweep: row sizes vary so the
// 8000-byte front cut lands at every offset of the key name and value of a
// secret row, the 20-line tail boundary lands on every line of a
// pretty-printed record, and the capture cap lands at every offset of a
// row; no 6-character run of any secret reaches the client.
func TestFailureDetailCutSweep(t *testing.T) {
	t.Setenv(cliutil.MCPReadOnlyEnvVar, "1")
	hit := map[int]bool{}
	for filler := 0; filler < 2000; filler++ {
		rows := make([]string, 25)
		secrets := make([]string, 25)
		for i := range rows {
			rows[i] = networkRow(i, 403-len(networkRow(i, 0)))
			secrets[i] = cutSecret(i)
		}
		rows[24] = networkRow(24, filler)
		tail := strings.Join(rows[5:], "\n")
		cut := len(tail) - failedStdoutDetailBytes
		keyAt := strings.Index(rows[5], `"secret_key"`)
		end := strings.Index(rows[5], cutSecret(5)) + len(cutSecret(5))
		if cut < keyAt || cut > end {
			continue
		}
		hit[cut-keyAt] = true
		out := failureText(strings.Join(rows, "\n")+"\n", "panic: boom")
		if s := leaksAnySecret(out, secrets); s != "" {
			t.Fatalf("front cut at key offset %d leaked %s: %.500s", cut-keyAt, s, out)
		}
	}
	if want := len(`"secret_key":"`) + len(cutSecret(5)); len(hit) < want {
		t.Fatalf("the sweep reached %d of %d offsets in the key and value", len(hit), want)
	}

	// The 20-line tail boundary on every line of a pretty-printed record.
	secret := cutSecret(42)
	record := []string{"{", `  "id": 42,`, `  "name": "Site 42",`, `  "secret_key":`, `    "` + secret + `",`, `  "client_secret": [`, `    "` + cutSecret(43) + `"`, `  ]`, "}"}
	for after := 0; after < 25; after++ {
		var lines []string
		lines = append(lines, record...)
		for i := 0; i < after; i++ {
			lines = append(lines, fmt.Sprintf(`{"event":"progress","n":%d}`, i))
		}
		out := failureText(strings.Join(lines, "\n"), "Error: failed")
		if s := leaksAnySecret(out, []string{secret, cutSecret(43)}); s != "" {
			t.Fatalf("tail boundary with %d trailing lines leaked: %.500s", after, out)
		}
	}

	// The capture cap at every offset of a secret row.
	var rows []string
	for i := 0; i < 6; i++ {
		rows = append(rows, networkRow(i, 30))
	}
	full := strings.Join(rows, "\n")
	start := len(full) - len(rows[5])
	for n := start; n <= len(full); n++ {
		out := failureText(full[:n], "Error: failed")
		if s := leaksAnySecret(out, []string{cutSecret(0), cutSecret(1), cutSecret(2), cutSecret(3), cutSecret(4), cutSecret(5)}); s != "" {
			t.Fatalf("capture cap at byte %d leaked %s: %.500s", n, s, out)
		}
	}
}
