// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package mcp

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"

	"eset-protect-pp-cli/internal/client"
	"eset-protect-pp-cli/internal/cliutil"
	"eset-protect-pp-cli/internal/cliutil/credvariants"
	mcplib "github.com/mark3labs/mcp-go/mcp"
)

// executeSample holds every byte a text path would damage: ESC, OSC 52, BEL,
// bare CR, C1 CSI (both encodings), NUL and bytes that are not UTF-8.
var executeSample = []byte("PK\x03\x04\x1b]52;c;AAA\x07\r\x1b[2K\xc2\x9b\x9b\n\r\n\x00\xff\xfeAAAAAAAAAAAAAAAA")

const executeErrorSentinel = "MCPDLSECRET-2R7N"

func executeCall(t *testing.T, id string) *mcplib.CallToolResult {
	t.Helper()
	res, err := handleCodeOrchExecute(context.Background(), mcplib.CallToolRequest{Params: mcplib.CallToolParams{Arguments: map[string]any{"endpoint_id": id, "params": map[string]any{}}}})
	if err != nil {
		t.Fatalf("%s: %v", id, err)
	}
	return res
}

// TestExecuteQuarantineDownloadsAreByteExact (adversarial round 5, P2): the
// eset-protect_execute registry entries for both quarantine downloads sent no
// binary marker, so a binary sample came back through the JSON path as lossy
// text (0xFF became U+FFFD) carrying the quarantined file's raw ESC, OSC 52,
// BEL and C1 bytes to the MCP host. In normal mode each download returns the
// _pp_binary envelope whose base64 decodes to exactly the server's bytes,
// under ESET's spec label (application/json) and under octet-stream; an HTTP
// 400 body naming a device token comes back redacted (Codex round 5, P1, the
// MCP twin of the CLI error path).
func TestExecuteQuarantineDownloadsAreByteExact(t *testing.T) {
	t.Setenv(cliutil.MCPReadOnlyEnvVar, "")
	contentType, status := "", http.StatusOK
	var sawMarker []string
	esetFakeServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get(client.BinaryResponseHeader) != "" {
			sawMarker = append(sawMarker, r.URL.Path)
		}
		if contentType != "" {
			w.Header().Set("Content-Type", contentType)
		}
		w.WriteHeader(status)
		if status != http.StatusOK {
			_, _ = w.Write([]byte(`{"code":3,"message":"bad","details":[{"deviceToken":"` + executeErrorSentinel + `"}]}`))
			return
		}
		_, _ = w.Write(executeSample)
	})
	ids := []string{"quarantined-objects-download.quarantined-objects", "quarantined-objects-batch-download.quarantined-objects"}
	for _, ct := range []string{"application/octet-stream", "application/json", ""} {
		contentType, status = ct, http.StatusOK
		for _, id := range ids {
			res := executeCall(t, id)
			text := toolText(t, res)
			if res.IsError {
				t.Fatalf("%s (Content-Type %q): %s", id, ct, text)
			}
			var env struct {
				PPBinary bool   `json:"_pp_binary"`
				Encoding string `json:"encoding"`
				Data     string `json:"data"`
			}
			if err := json.Unmarshal([]byte(text), &env); err != nil || !env.PPBinary || env.Encoding != "base64" {
				t.Fatalf("%s (Content-Type %q) is not the binary envelope: %q", id, ct, text)
			}
			got, err := base64.StdEncoding.DecodeString(env.Data)
			if err != nil || !bytes.Equal(got, executeSample) {
				t.Fatalf("%s (Content-Type %q): the envelope is not the server's bytes:\n got %q\nwant %q", id, ct, got, executeSample)
			}
			if strings.ContainsAny(text, "\x1b\x07\r\x00") || strings.Contains(text, "\ufffd") || strings.Contains(text, "\u009b") {
				t.Fatalf("%s (Content-Type %q): raw control bytes reached the MCP host: %q", id, ct, text)
			}
		}
	}
	if len(sawMarker) != 0 {
		t.Fatalf("the internal binary marker went out on the wire: %v", sawMarker)
	}
	contentType, status = "application/json", http.StatusBadRequest
	for _, id := range ids {
		text := toolText(t, executeCall(t, id))
		if credvariants.Recoverable([]byte(text), executeErrorSentinel) {
			t.Fatalf("%s: an HTTP 400 body's device token reached the MCP result: %s", id, text)
		}
		if !strings.Contains(text, "400") {
			t.Fatalf("%s: the error no longer names the status: %s", id, text)
		}
	}
}

var (
	binaryMarkerRE = regexp.MustCompile(`"X-Printing-Press-Binary-Response":\s*"true"`)
	ppEndpointRE   = regexp.MustCompile(`"pp:endpoint":\s*"([^"]+)"`)
)

// TestExecuteDeclaresBinaryExactlyWhereTheCLIDoes scans the CLI command
// sources: every command that sends the binary marker names its endpoint id
// in its pp:endpoint annotation, and the execute registry must carry the
// marker on exactly those ids, so a reprint that adds or drops a binary
// endpoint cannot leave the two surfaces disagreeing.
func TestExecuteDeclaresBinaryExactlyWhereTheCLIDoes(t *testing.T) {
	files, err := filepath.Glob(filepath.Join("..", "cli", "*.go"))
	if err != nil || len(files) == 0 {
		t.Fatalf("no CLI sources: %v", err)
	}
	var cliIDs []string
	for _, f := range files {
		if strings.HasSuffix(f, "_test.go") {
			continue
		}
		b, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		if !binaryMarkerRE.Match(b) {
			continue
		}
		m := ppEndpointRE.FindSubmatch(b)
		if m == nil {
			t.Fatalf("%s sends the binary marker but names no pp:endpoint", f)
		}
		cliIDs = append(cliIDs, string(m[1]))
	}
	var registryIDs []string
	for _, ep := range codeOrchEndpoints {
		for k, v := range ep.HeaderOverrides {
			if strings.EqualFold(k, client.BinaryResponseHeader) && strings.EqualFold(v, "true") {
				registryIDs = append(registryIDs, ep.ID)
			}
		}
	}
	sort.Strings(cliIDs)
	sort.Strings(registryIDs)
	if len(cliIDs) == 0 || strings.Join(cliIDs, ",") != strings.Join(registryIDs, ",") {
		t.Fatalf("binary endpoints differ:\n CLI      %v\n execute  %v", cliIDs, registryIDs)
	}
}
