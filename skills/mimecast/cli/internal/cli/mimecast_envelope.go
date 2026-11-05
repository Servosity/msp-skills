// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-written: Mimecast API 2.0 request/response envelope helpers shared by
// the novel workflow commands (why, held triage/release-safe/purge-sender/
// release-audit, senders suggest, mailflow, ttp who-clicked).

package cli

import (
	"cmp"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"mimecast-pp-cli/internal/client"
	"mimecast-pp-cli/internal/cliutil"
)

// printNovelJSON prints a hand-shaped novel view. Every field the view emits
// is part of the answer, and a sparse one (clicked_through_warning on one
// click in many, a held_id on one event) is often the signal, so --agent and
// --compact keep all of them.
func printNovelJSON(w io.Writer, view any, flags *rootFlags) error {
	raw, err := json.Marshal(view)
	if err != nil {
		return err
	}
	return printJSONFilteredKeep(w, json.RawMessage(raw), flags, novelKeys(raw)...)
}

// novelKeys is every object key in a view, at any depth: the compaction keep
// floor for a novel view.
func novelKeys(raw []byte) []string {
	var tree any
	if json.Unmarshal(raw, &tree) != nil {
		return nil
	}
	seen := map[string]bool{}
	var walk func(any)
	walk = func(v any) {
		switch t := v.(type) {
		case map[string]any:
			for k, x := range t {
				seen[k] = true
				walk(x)
			}
		case []any:
			for _, x := range t {
				walk(x)
			}
		}
	}
	walk(tree)
	keys := make([]string, 0, len(seen))
	for k := range seen {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	return keys
}

// mcTimeLayout is Mimecast's documented date format: yyyy-MM-dd'T'HH:mm:ssZ.
const mcTimeLayout = "2006-01-02T15:04:05-0700"

// mcMaxPageSize is the documented ceiling for meta.pagination.pageSize.
const mcMaxPageSize = 100

// mcFailure covers both failure shapes Mimecast returns: the gateway form
// {"code","message"} and the function form {"errors":[...],"key":{...}}.
type mcFailure struct {
	Code    string `json:"code,omitempty"`
	Message string `json:"message,omitempty"`
	Errors  []struct {
		Code      string `json:"code"`
		Message   string `json:"message"`
		Retryable bool   `json:"retryable"`
	} `json:"errors,omitempty"`
	Key json.RawMessage `json:"key,omitempty"`
}

// text joins every code/message pair in the failure.
func (f mcFailure) text() string {
	var parts []string
	if f.Code != "" || f.Message != "" {
		parts = append(parts, strings.TrimPrefix(f.Code+": "+f.Message, ": "))
	}
	for _, e := range f.Errors {
		parts = append(parts, strings.TrimPrefix(e.Code+": "+e.Message, ": "))
	}
	if len(parts) == 0 {
		return "request failed (no code or message)"
	}
	return strings.Join(parts, "; ")
}

// retryable reports whether every error in the failure is marked retryable.
func (f mcFailure) retryable() bool {
	if len(f.Errors) == 0 {
		return false
	}
	for _, e := range f.Errors {
		if !e.Retryable {
			return false
		}
	}
	return true
}

// keyIDs returns the id / ids the failed request item referred to.
func (f mcFailure) keyIDs() []string {
	var key struct {
		ID  string   `json:"id"`
		IDs []string `json:"ids"`
	}
	_ = json.Unmarshal(f.Key, &key)
	if key.ID != "" {
		return []string{key.ID}
	}
	return key.IDs
}

type mcResponse struct {
	Meta struct {
		Status     int `json:"status"`
		Pagination struct {
			PageSize int    `json:"pageSize"`
			Next     string `json:"next"`
			Previous string `json:"previous"`
		} `json:"pagination"`
	} `json:"meta"`
	Data []json.RawMessage `json:"data"`
	Fail []mcFailure       `json:"fail"`
}

func (r *mcResponse) failMessages() []string { return failTexts(r.Fail) }

func failTexts(fails []mcFailure) []string {
	out := make([]string, 0, len(fails))
	for _, f := range fails {
		if t := f.text(); t != "" {
			out = append(out, t)
		}
	}
	return out
}

// mcAccountCode resolves the delegated-access account code (MSP use): an
// explicit flag wins, then MIMECAST_ACCOUNT_CODE. The generated client also
// injects the env value into every envelope request.
func mcAccountCode(flagValue string) string {
	return cmp.Or(strings.TrimSpace(flagValue), client.MimecastAccountCode())
}

// mcBody builds {"meta":{...},"data":[...]}. pageSize 0 omits pagination.
func mcBody(items []any, pageSize int, pageToken, accountCode string) map[string]any {
	meta := map[string]any{}
	if pageSize > 0 || pageToken != "" {
		p := map[string]any{}
		if pageSize > 0 {
			p["pageSize"] = pageSize
		}
		if pageToken != "" {
			p["pageToken"] = pageToken
		}
		meta["pagination"] = p
	}
	if accountCode != "" {
		meta["accountCode"] = accountCode
	}
	if items == nil {
		items = []any{}
	}
	body := map[string]any{"data": items}
	if len(meta) > 0 {
		body["meta"] = meta
	}
	return body
}

func decodeMC(path string, raw json.RawMessage) (*mcResponse, error) {
	var resp mcResponse
	if err := json.Unmarshal(raw, &resp); err != nil {
		return nil, fmt.Errorf("decoding %s response: %w", path, err)
	}
	return &resp, nil
}

// mcRead sends a read-only envelope request (Mimecast reads ride POST).
func mcRead(ctx context.Context, c *client.Client, path string, body map[string]any) (*mcResponse, error) {
	raw, _, err := c.PostQueryWithParams(ctx, path, nil, body)
	if err != nil {
		return nil, err
	}
	resp, err := decodeMC(path, raw)
	if err != nil {
		return nil, err
	}
	// A read with any fail[] entry is not a complete answer; never let it
	// look like an empty result.
	if len(resp.Fail) > 0 {
		return resp, apiErr(fmt.Errorf("%s failed: %s", path, strings.Join(resp.failMessages(), "; ")))
	}
	return resp, nil
}

// mcWrite sends a state-changing envelope request. Partial failures come
// back in resp.Fail next to the successful items in resp.Data.
func mcWrite(ctx context.Context, c *client.Client, path string, body map[string]any) (*mcResponse, error) {
	raw, _, err := c.Post(ctx, path, body)
	if err != nil {
		return nil, err
	}
	return decodeMC(path, raw)
}

// mcPage is the outcome of a bounded paginated read.
type mcPage struct {
	Data      []json.RawMessage
	Pages     int
	Truncated bool // the scan did not reach the last page
	Loop      bool // it stopped because Mimecast repeated a page token
}

// note explains an incomplete scan ("" when it finished). advice is what the
// caller can do about a page-cap stop; a pagination loop gets its own text
// because raising the cap would only loop again.
func (p mcPage) note(advice string) string {
	switch {
	case p.Loop:
		return fmt.Sprintf("Mimecast repeated a page token after %d pages, so the scan stopped early; narrow the time window to see the rest", p.Pages)
	case p.Truncated:
		return fmt.Sprintf("scan stopped at %d pages; %s", p.Pages, advice)
	}
	return ""
}

// mcReadAll follows meta.pagination.next for up to maxPages pages. It stops
// on a repeated page token (a known Mimecast pagination loop).
func mcReadAll(ctx context.Context, c *client.Client, path string, items []any, accountCode string, pageSize, maxPages int) (mcPage, error) {
	return mcReadUntil(ctx, c, path, items, accountCode, pageSize, maxPages, nil)
}

// mcReadUntil is mcReadAll that also stops once done reports the rows read
// so far are enough (done may be nil).
func mcReadUntil(ctx context.Context, c *client.Client, path string, items []any, accountCode string, pageSize, maxPages int, done func([]json.RawMessage) bool) (mcPage, error) {
	if pageSize <= 0 || pageSize > mcMaxPageSize {
		pageSize = mcMaxPageSize
	}
	if maxPages <= 0 {
		maxPages = 1
	}
	var out mcPage
	seen := map[string]bool{}
	seenPage := map[[32]byte]bool{}
	token := ""
	for out.Pages < maxPages {
		resp, err := mcRead(ctx, c, path, mcBody(items, pageSize, token, accountCode))
		if err != nil {
			return out, err
		}
		out.Pages++
		// A looping token re-serves the same page; keep only its first copy.
		raw, _ := json.Marshal(resp.Data)
		if sig := sha256.Sum256(raw); !seenPage[sig] {
			seenPage[sig] = true
			out.Data = append(out.Data, resp.Data...)
		}
		token = resp.Meta.Pagination.Next
		if token == "" || (done != nil && done(out.Data)) {
			return out, nil
		}
		if seen[token] {
			out.Truncated, out.Loop = true, true // pagination loop: the scan did not reach the end
			return out, nil
		}
		seen[token] = true
	}
	out.Truncated = true // the loop only exits here with a next token pending
	return out, nil
}

// unwrapList pulls a nested list such as data[].heldEmails out of the
// data array. Rows whose wrapper key is absent are treated as list items.
func unwrapList(data []json.RawMessage, key string) []json.RawMessage {
	out := make([]json.RawMessage, 0, len(data))
	for _, d := range data {
		var wrapper map[string]json.RawMessage
		if json.Unmarshal(d, &wrapper) == nil {
			if inner, ok := wrapper[key]; ok {
				var items []json.RawMessage
				if json.Unmarshal(inner, &items) == nil {
					out = append(out, items...)
				}
				continue
			}
		}
		out = append(out, d)
	}
	return out
}

// mimecastPartialFailure lets the generated mutate path recognize Mimecast's
// fail[] array next to successful data[] items (for example 2 of 3 releases).
func mimecastPartialFailure(top map[string]any) *partialFailureReport {
	fails, ok := top["fail"].([]any)
	if !ok || len(fails) == 0 {
		return nil
	}
	raw, _ := json.Marshal(fails)
	var parsed []mcFailure
	_ = json.Unmarshal(raw, &parsed)
	msgs := failTexts(parsed)
	succeeded := 0
	if data, ok := top["data"].([]any); ok {
		succeeded = len(data)
	}
	return &partialFailureReport{
		Field:   "fail",
		Message: fmt.Sprintf("%d item(s) failed, %d succeeded: %s", len(fails), succeeded, strings.Join(msgs, "; ")),
		Details: fails,
	}
}

// mcTime formats t in Mimecast's request format (UTC).
func mcTime(t time.Time) string { return t.UTC().Format(mcTimeLayout) }

// parseMCTime parses Mimecast response timestamps (with or without zone colon).
func parseMCTime(s string) (time.Time, bool) {
	s = strings.TrimSpace(s)
	for _, layout := range []string{mcTimeLayout, time.RFC3339, "2006-01-02T15:04:05.000-0700", "2006-01-02T15:04:05Z0700"} {
		if t, err := time.Parse(layout, s); err == nil {
			return t, true
		}
	}
	return time.Time{}, false
}

// parseSince accepts a lookback ("90m", "24h", "7d", "2w") or an absolute
// RFC3339 / Mimecast timestamp / date and returns the start time.
func parseSince(s string, now time.Time) (time.Time, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return time.Time{}, fmt.Errorf("empty --since")
	}
	for _, layout := range []string{time.RFC3339, mcTimeLayout, "2006-01-02"} {
		if t, err := time.Parse(layout, s); err == nil {
			return t, nil
		}
	}
	unit := s[len(s)-1]
	n, err := strconv.Atoi(s[:len(s)-1])
	if err != nil || n < 0 {
		return time.Time{}, fmt.Errorf("invalid --since %q: use a lookback like 24h or 7d, or an RFC3339 time", s)
	}
	switch unit {
	case 'm':
		return now.Add(-time.Duration(n) * time.Minute), nil
	case 'h':
		return now.Add(-time.Duration(n) * time.Hour), nil
	case 'd':
		return now.AddDate(0, 0, -n), nil
	case 'w':
		return now.AddDate(0, 0, -7*n), nil
	}
	return time.Time{}, fmt.Errorf("invalid --since %q: unit must be m, h, d or w", s)
}

// mcAddr is Mimecast's {displayableName, emailAddress} pair.
type mcAddr struct {
	DisplayableName string `json:"displayableName,omitempty"`
	EmailAddress    string `json:"emailAddress,omitempty"`
}

// addrMatches reports whether address satisfies want: an exact address
// (case-insensitive) or, when want has no "@", the address's domain or a
// subdomain of it.
func addrMatches(address, want string) bool {
	address = strings.ToLower(strings.TrimSpace(address))
	want = strings.ToLower(strings.TrimSpace(strings.TrimPrefix(want, "@")))
	if address == "" || want == "" {
		return false
	}
	if strings.Contains(want, "@") {
		return address == want
	}
	d := addrDomain(address)
	return d == want || strings.HasSuffix(d, "."+want)
}

// addrDomain returns the lowercased domain part of an address.
func addrDomain(address string) string {
	address = strings.ToLower(strings.TrimSpace(address))
	if i := strings.LastIndex(address, "@"); i >= 0 {
		return address[i+1:]
	}
	return address
}

// Release-risk classes for held messages. The rule table is mechanical and
// errs toward caution: a threat-type hold is never released on request.
const (
	riskNeverRelease = "never-release" // impersonation, malware, sandbox, phishing, URL/attachment protect
	riskLikelyJunk   = "likely-junk"   // spam/graymail/bulk holds with a high spam score
	riskPolicyReview = "policy-review" // content, DLP, attachment-type, size or recipient policies
	riskReview       = "review"        // anything else: a human decides
)

var neverReleaseTerms = []string{"impersonation", "malware", "virus", "sandbox", "malicious", "phish", "ransomware", "attachment protect", "url protect", "targeted threat", "dangerous file", "spoof"}
var junkTerms = []string{"spam", "graymail", "greymail", "bulk"}
var policyTerms = []string{"content", "dlp", "data leak", "attachment", "size", "recipient", "encrypt", "policy", "keyword", "lexicon"}

// classifyHold returns the release-risk class and the matched rule. It is a
// keyword heuristic over free-text hold reason and policy names, not verified
// against every Mimecast hold reason. spamScore < 0 means unknown.
func classifyHold(reason, policy, spamVerdict string, spamScore int) (string, string) {
	text := strings.ToLower(reason + " " + policy)
	for _, t := range neverReleaseTerms {
		if strings.Contains(text, t) {
			return riskNeverRelease, "matched '" + t + "'"
		}
	}
	for _, t := range junkTerms {
		if strings.Contains(text, t) {
			verdict := strings.ToLower(spamVerdict)
			if spamScore >= 28 || strings.Contains(verdict, "spam") || strings.Contains(verdict, "high") {
				return riskLikelyJunk, "matched '" + t + "' with high spam signal"
			}
			return riskReview, "matched '" + t + "' with low or unknown spam score"
		}
	}
	for _, t := range policyTerms {
		if strings.Contains(text, t) {
			return riskPolicyReview, "matched '" + t + "'"
		}
	}
	return riskReview, "no rule matched"
}

// riskRank orders classes from most to least dangerous to release.
func riskRank(class string) int {
	switch class {
	case riskNeverRelease:
		return 0
	case riskPolicyReview:
		return 1
	case riskReview:
		return 2
	case riskLikelyJunk:
		return 3
	}
	return 2
}

// batches splits ids into chunks of at most size.
func batches(ids []string, size int) [][]string {
	return slices.Collect(slices.Chunk(ids, size))
}

// shellQuote single-quotes s for a POSIX shell so API-supplied values (sender
// addresses, IDs) cannot break out of a suggested command.
func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'"'"'`) + "'"
}

// mcWriteOutcomeErr is the exit error for a write command that finished with
// failures: it keeps the first transport error's class (429 exits 7, 401/403
// exit 4) and falls back to a generic API error (exit 5).
func mcWriteOutcomeErr(summary error, firstCause error) error {
	if firstCause != nil {
		return classifyAPIErrorOnly(fmt.Errorf("%v: %w", summary, firstCause))
	}
	return apiErr(summary)
}

// REST endpoints (anything outside /api/) have no delegated-account selector,
// so with MIMECAST_ACCOUNT_CODE set they would act on the API application's
// own account. This guard, installed on every client (CLI and MCP), refuses
// such writes and warns once on reads.
func init() {
	// MIMECAST_ACCOUNT_CODE is optional (MSP delegate access); an unfilled
	// MCPB placeholder counts as unset.
	client.SetAccountCodeSource(func() string { return cliutil.EnvOverride("MIMECAST_ACCOUNT_CODE") })
	registerClientHook(func(c *client.Client) error {
		// The client package always sets the kit-wrapped transport (credkit-wiring).
		if c.HTTPClient != nil && c.HTTPClient.Transport != nil {
			c.HTTPClient.Transport = &accountScopeTransport{base: c.HTTPClient.Transport}
		}
		return nil
	})
}

type accountScopeTransport struct {
	base   http.RoundTripper
	warned sync.Once
}

func (t *accountScopeTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	if code := client.MimecastAccountCode(); code != "" && !strings.HasPrefix(r.URL.Path, "/api/") && !strings.HasPrefix(r.URL.Path, "/oauth/") {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			return nil, configErr(fmt.Errorf("MIMECAST_ACCOUNT_CODE is set, but %s %s has no delegated-account selector and would change the API application's own account; unset MIMECAST_ACCOUNT_CODE to run it", r.Method, r.URL.Path))
		}
		t.warned.Do(func() {
			fmt.Fprintf(os.Stderr, "warning: %s reads the API application's own account; MIMECAST_ACCOUNT_CODE applies only to /api/ endpoints\n", r.URL.Path)
		})
	}
	return t.base.RoundTrip(r)
}
