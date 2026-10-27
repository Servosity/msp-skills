// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package client

import (
	"context"
	"encoding/base32"
	"encoding/base64"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"sentinelone-pp-cli/internal/cliutil"
	"sentinelone-pp-cli/internal/config"
)

const successFunnelHeldValue = "abcdefgh12345678"

func successFunnelClient(t *testing.T, body string, contentType string) *Client {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", contentType)
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(server.Close)
	c := New(&config.Config{BaseURL: server.URL, SentineloneApiToken: successFunnelHeldValue}, 5*time.Second, 0)
	c.HTTPClient = server.Client()
	c.NoCache = true
	return c
}

// TestSuccessAnswerEchoingTheTokenIsWithheld: an HTTP 200 whose body repeats
// the API token, in any encoding the shared funnel decodes, must never reach a
// caller (the CLI's stdout, an MCP tool result, or the local store).
func TestSuccessAnswerEchoingTheTokenIsWithheld(t *testing.T) {
	b64 := base64.StdEncoding.EncodeToString([]byte(successFunnelHeldValue))
	for name, body := range map[string]string{
		"plain":       `{"data":{"message":"` + successFunnelHeldValue + `"}}`,
		"json-escape": `{"data":{"message":"abcdefgh12345678"}}`,
		"base64":      `{"data":[{"note":"` + b64 + `"}]}`,
		"percent":     `{"data":{"next":"https://x.example/?t=` + url.QueryEscape("ApiToken "+successFunnelHeldValue) + `"}}`,
		"in-a-key":    `{"data":{"` + successFunnelHeldValue + `":1}}`,
		"text-body":   "token=" + successFunnelHeldValue,
	} {
		t.Run(name, func(t *testing.T) {
			ct := "application/json"
			if name == "text-body" {
				ct = "text/plain"
			}
			c := successFunnelClient(t, body, ct)
			got, err := c.Get(context.Background(), "/agents", nil)
			if strings.Contains(string(got), successFunnelHeldValue) || strings.Contains(string(got), b64) {
				t.Fatalf("success body carrying the token reached the caller: %s", got)
			}
			if err == nil {
				t.Fatalf("an answer that repeats the token must fail closed, got body %s", got)
			}
			if strings.Contains(err.Error(), successFunnelHeldValue) {
				t.Fatalf("error text repeats the token: %v", err)
			}
		})
	}
}

// TestSuccessAnswerWithoutTheTokenPassesUnchanged is the other direction: an
// ordinary answer, including one with long base64-looking ids, is untouched.
func TestSuccessAnswerWithoutTheTokenPassesUnchanged(t *testing.T) {
	body := `{"data":[{"id":"225494730938493804","uuid":"ff819e70af13be381993075eb0ce5f2f6de05b11","computerName":"ACME-WS-01","note":"aGVsbG8gd29ybGQgdGhpcyBpcyBub3QgYSBzZWNyZXQ="}],"pagination":{"nextCursor":null}}`
	c := successFunnelClient(t, body, "application/json")
	got, err := c.Get(context.Background(), "/agents", nil)
	if err != nil {
		t.Fatalf("clean answer refused: %v", err)
	}
	if !strings.Contains(string(got), "ACME-WS-01") {
		t.Fatalf("clean answer altered: %s", got)
	}
}

// TestBinaryAnswerEchoingTheTokenIsWithheld: a binary answer is returned as a
// base64 envelope, which would hide an echoed token from a text check.
func TestBinaryAnswerEchoingTheTokenIsWithheld(t *testing.T) {
	utf16 := make([]byte, 0, 2*len(successFunnelHeldValue))
	for _, r := range successFunnelHeldValue {
		utf16 = append(utf16, byte(r), 0)
	}
	for name, body := range map[string]string{
		"ascii": "PK\x03\x04\x00\x01junk " + successFunnelHeldValue + "\x00\x02",
		"utf16": "\xff\xfe" + string(utf16) + "\x00\x00",
	} {
		t.Run(name, func(t *testing.T) {
			c := successFunnelClient(t, body, "application/octet-stream")
			got, err := c.Get(context.Background(), "/agents/1/uploads/1", nil)
			if err == nil {
				t.Fatalf("a binary answer carrying the token must fail closed, got %s", got)
			}
		})
	}
}

// TestLargeCleanPagePasses: a page of 3000 agents (several MB, well over the
// funnel's 1 MiB single-pass cap) is checked string by string and passes.
func TestLargeCleanPagePasses(t *testing.T) {
	var b strings.Builder
	b.WriteString(`{"data":[`)
	for i := 0; i < 3000; i++ {
		if i > 0 {
			b.WriteString(",")
		}
		fmt.Fprintf(&b, `{"id":"2254947309384%05d","uuid":"ff819e70af13be381993075eb0ce5f2f6de%05d","computerName":"ACME-WS-%05d","siteName":"Acme Dental","siteId":"225494730938493804","osName":"Windows 11 Pro","agentVersion":"24.1.2.6","networkStatus":"connected","lastActiveDate":"2026-10-24T20:00:00.123456Z","networkInterfaces":[{"inet":["10.1.%d.%d"],"physical":"aa:bb:cc:dd:%02x:%02x","name":"Ethernet"}],"groupName":"Default Group","domain":"acme.local","externalIp":"203.0.113.7","mitigationMode":"protect"}`, i, i, i, i/250, i%250, i/256, i%256)
	}
	b.WriteString(`],"pagination":{"nextCursor":null}}`)
	body := b.String()
	if len(body) < 1<<20 {
		t.Fatalf("fixture too small: %d bytes", len(body))
	}
	c := successFunnelClient(t, body, "application/json")
	start := time.Now()
	got, err := c.Get(context.Background(), "/agents", nil)
	if err != nil {
		t.Fatalf("clean %d-byte page refused: %v", len(body), err)
	}
	if len(got) < len(body)/2 {
		t.Fatalf("page truncated: %d bytes", len(got))
	}
	t.Logf("checked %d bytes in %s", len(body), time.Since(start))
}

// successFunnelAgentsPage is an ordinary SentinelOne /agents page of n agents.
func successFunnelAgentsPage(n int) string {
	var b strings.Builder
	b.WriteString(`{"data":[`)
	for i := 0; i < n; i++ {
		if i > 0 {
			b.WriteString(",")
		}
		fmt.Fprintf(&b, `{"id":"2254947309384%05d","uuid":"ff819e70af13be381993075eb0ce5f2f6de%05d","computerName":"ACME-WS-%05d","siteName":"Acme Dental","siteId":"225494730938493804","osName":"Windows 11 Pro","agentVersion":"24.1.2.6","networkStatus":"connected","lastActiveDate":"2026-10-24T20:00:00.123456Z","networkInterfaces":[{"inet":["10.1.%d.%d"],"physical":"aa:bb:cc:dd:%02x:%02x","name":"Ethernet"}],"groupName":"Default Group","domain":"acme.local","externalIp":"203.0.113.7","mitigationMode":"protect"}`, i, i, i, i/250, i%250, i/256, i%256)
	}
	b.WriteString(`],"pagination":{"nextCursor":null,"totalItems":` + fmt.Sprint(n) + `}}`)
	return b.String()
}

// successFunnelProse is n bytes of ordinary English text (a log or CSV export
// body): normalizing it joins whole sentences into long Base32-alphabet runs,
// the worst case for the decode budget.
func successFunnelProse(n int) string {
	const line = "Agent ACME-WS-01 reported a suspicious process; the policy mitigated it and the analyst marked the threat as resolved, notes attached.\n"
	return strings.Repeat(line, n/len(line)+1)[:n]
}

// successFunnelEscaped is n bytes of an ordinary escape-heavy page (HTML
// entities and tags, Windows paths, percent-encoded URLs, quoted-printable and
// %u escapes), where every escape decoder changes the text.
func successFunnelEscaped(n int) string {
	const line = `<tr><td class="path">C:\Windows\System32\cmd.exe /c "echo &amp; dir C:\temp\new"</td><td><a href="https://usea1.sentinelone.net/incidents/threats?filter=status%3Dactive%26q%3Dcmd%2Bexe">open</a> =3D &lt;b&gt; \u0041 %u0042 &#65;</td></tr>` + "\n"
	return strings.Repeat(line, n/len(line)+1)[:n]
}

// successFunnelNoise is n bytes of deterministic pseudo-random binary (a zip or
// an image).
func successFunnelNoise(n int) string {
	b := make([]byte, n)
	x := uint32(2463534242)
	for i := range b {
		x ^= x << 13
		x ^= x >> 17
		x ^= x << 5
		b[i] = byte(x)
	}
	return string(b)
}

// TestSuccessFunnelClosesEveryEncodingClass: a 2xx (or cached) answer that
// repeats a held credential in ANY form the check can normalize or decode is
// withheld, whatever its shape: Base32 in either alphabet or case, a
// duplicate JSON key, a value split by whitespace or an invisible character,
// a binary body split across printable runs, or a value too large to decode.
// The other direction holds too: a clean page passes, however large, as long
// as no single value is over the decoder's cap.
func TestSuccessFunnelClosesEveryEncodingClass(t *testing.T) {
	const digitToken = "12345678901234567890"
	cliutil.FunnelHold(digitToken, successFunnelHeldValue)
	tok := []byte(successFunnelHeldValue)
	b32 := base32.StdEncoding.EncodeToString(tok)
	b32Hex := base32.HexEncoding.EncodeToString(tok)
	b32HexRawLower := strings.ToLower(base32.HexEncoding.WithPadding(base32.NoPadding).EncodeToString(tok))
	b32RawLower := strings.ToLower(base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(tok))
	// The token at byte offset 3 of a longer blob: its own Base32 form is not a
	// substring of the blob's, so only decoding the blob finds it.
	b32Offset := base32.StdEncoding.EncodeToString([]byte("id=" + successFunnelHeldValue + ";v=2"))
	overCap := strings.Repeat("x", (1<<20)+1)
	// Base32 wrapped in a form the canonical funnel decodes, and the reverse.
	b64OfB32 := base64.StdEncoding.EncodeToString([]byte(b32))
	b32OfB64 := base32.StdEncoding.EncodeToString([]byte(base64.StdEncoding.EncodeToString(tok)))
	var pctB32, entB32 strings.Builder
	for i := 0; i < len(b32); i++ {
		fmt.Fprintf(&pctB32, "%%%02X", b32[i])
		fmt.Fprintf(&entB32, "&#%d;", b32[i])
	}

	for _, tc := range []struct {
		name   string
		body   string
		binary bool
		want   bool
	}{
		// The three Codex inputs, verbatim.
		{"codex1-base32-of-token", `{"echo":"GEZDGNBVGY3TQOJQGEZDGNBVGY3TQOJQ"}`, false, true},
		{"codex2-duplicate-key-first-value", `{"echo":"abcdefgh12345678","echo":"clean"}`, false, true},
		{"codex3-binary-crlf-split", "\x89PNG\x00\x01abcdefgh\r\n12345678\x00\x02", true, true},
		// The rest of the class.
		{"base32-std-padded", `{"data":{"note":"` + b32 + `"}}`, false, true},
		{"base32-hex-alphabet", `{"data":{"note":"` + b32Hex + `"}}`, false, true},
		{"base32-hex-lower-unpadded", `{"data":{"note":"` + b32HexRawLower + `"}}`, false, true},
		{"base32-lower-unpadded", `{"data":{"note":"` + b32RawLower + `"}}`, false, true},
		{"base32-glued-to-a-label", `{"data":{"note":"ID` + b32RawLower + `"}}`, false, true},
		{"base32-token-at-an-offset", `{"data":{"note":"` + b32Offset + `"}}`, false, true},
		{"base32-split-by-whitespace", `{"data":{"note":"` + b32[:10] + ` \n ` + b32[10:] + `"}}`, false, true},
		{"base32-of-token-split-by-an-invalid-byte", `{"data":{"note":"` + base32.StdEncoding.EncodeToString([]byte("abcdefgh\xff12345678")) + `"}}`, false, true},
		{"base64-of-base32", `{"data":{"note":"` + b64OfB32 + `"}}`, false, true},
		{"base32-of-base64", `{"data":{"note":"` + b32OfB64 + `"}}`, false, true},
		{"percent-encoded-base32", `{"data":{"next":"https://x.example/?t=` + pctB32.String() + `"}}`, false, true},
		{"html-entity-base32", `<p>` + entB32.String() + `</p>`, false, true},
		{"percent-base32-inside-an-escaped-page", successFunnelEscaped(16<<10) + pctB32.String() + successFunnelEscaped(16<<10), false, true},
		{"base32-split-by-tags", `<p>` + b32[:12] + `<b></b>` + b32[12:] + `</p>`, false, true},
		{"duplicate-key-later-value", `{"echo":"clean","echo":"abcdefgh12345678"}`, false, true},
		{"zero-width-split-json-raw", `{"data":{"note":"abcdefgh` + "\u200b" + `12345678"}}`, false, true},
		{"zero-width-split-json-escaped", `{"data":{"note":"abcdefgh\u200b12345678"}}`, false, true},
		{"nbsp-split-json", `{"data":{"note":"abcdefgh` + "\u00a0" + `12345678"}}`, false, true},
		{"tab-split-text", "result: abcdefgh\t12345678 done", false, true},
		{"space-split-text", "result: abcdefgh 12345678 done", false, true},
		{"invalid-json-is-text", `{"echo":"abcdefgh12345678"`, false, true},
		{"binary-zero-width-split", "\x00\x01abcdefgh" + "\u200b" + "12345678\x02", true, true},
		{"binary-token-split-by-an-invalid-byte", "\x00\x01 key abcdefgh\xff12345678 \x02\x03", true, true},
		{"binary-base32", "\x00\x01" + b32 + "\x02", true, true},
		{"rot13-of-token", `{"data":{"note":"nopqrstu12345678"}}`, false, true},
		{"value-over-cap-json", `{"data":{"blob":"` + overCap + `"}}`, false, true},
		{"value-over-cap-text", overCap, false, true},
		{"value-over-cap-binary", "\x00" + overCap, true, true},
		// The other direction.
		{"clean-agents-page", successFunnelAgentsPage(3), false, false},
		{"large-clean-page-many-small-values", successFunnelAgentsPage(3000), false, false},
		{"clean-text", "ok: 3 agents updated", false, false},
		{"clean-binary", "\x89PNG\r\n\x1a\n\x00\x00\x00\rIHDR\x00\x00\x01\x00", true, false},
		{"short-value", `{"a":"abc"}`, false, false},
		{"large-clean-text-under-cap", successFunnelProse(900 << 10), false, false},
		{"large-clean-binary-under-cap", successFunnelNoise(900 << 10), true, false},
		{"large-clean-escaped-page", successFunnelEscaped(200 << 10), false, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if tc.name == "large-clean-page-many-small-values" && len(tc.body) <= 1<<20 {
				t.Fatalf("fixture must exceed the decoder cap: %d bytes", len(tc.body))
			}
			start := time.Now()
			got := successEchoesHeld([]byte(tc.body), tc.binary)
			if got != tc.want {
				t.Fatalf("successEchoesHeld(%.80q, binary=%v) = %v, want %v", tc.body, tc.binary, got, tc.want)
			}
			if d := time.Since(start); d > 2*time.Second {
				t.Logf("%s took %s for %d bytes", tc.name, d, len(tc.body))
			}
		})
	}

	// The needle stage: a form the funnel cannot decode back (the token with its
	// case changed) is matched against every encoded form client.go computes for
	// the client's own credentials, on the real success path.
	for name, body := range map[string]string{
		"upper-cased-token":       `{"data":{"note":"` + strings.ToUpper(successFunnelHeldValue) + `"}}`,
		"upper-cased-in-a-string": `{"data":{"note":"key ` + strings.ToUpper(successFunnelHeldValue) + ` rotated"}}`,
	} {
		t.Run("client-"+name, func(t *testing.T) {
			c := successFunnelClient(t, body, "application/json")
			got, err := c.Get(context.Background(), "/agents", nil)
			if err == nil {
				t.Fatalf("a success answer repeating the token in another case must fail closed, got %s", got)
			}
		})
	}
}
