// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package credkit

// Contract section 2, public text: fixed text a connector emits whether or
// not it holds any credential, exactly the entries of publicText. It is
// used only for collisions and exempts no piece: a startup credential that
// collides refuses startup, naming its source only; a value acquired while
// running that occurs in an entry as written is not held, and is recorded
// naming its source; a held value that partly overlaps an entry withholds
// the unit that carries it, like any piece.

import (
	"fmt"
	"net/http"
	"os"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"unicode"
	"unicode/utf8"
)

// ExitRefused is the exit status when a startup credential collides with
// public text.
const ExitRefused = 78

func refusalText(src Source) string {
	if src == "" {
		return "[credkit: refusing to start: a startup credential occurs in public text]"
	}
	return "[credkit: refusing to start: a startup credential occurs in public text: " + src + "]"
}

func recordText(src Source) string {
	if src == "" {
		return "[credkit: not held: a value acquired while running occurs in public text]"
	}
	return "[credkit: not held: a value acquired while running occurs in public text: " + src + "]"
}

// publicText is the table, by the contract's kinds.
var publicText = sync.OnceValue(func() []string {
	var t []string
	// 1. The kit's own notices (the withhold notices, the header
	// replacement, which is the MCP notice, the refusal and the record),
	// without the names filled in at run time.
	for _, s := range []string{SinkStdout, SinkStderr, SinkMCP, SinkSQLite, SinkFile, SinkHTTP, SinkChild, "this sink"} {
		t = append(t, Notice(s))
	}
	t = append(t, refusalText(""), recordText(""))
	// 2. HTTP: the version tokens, net/http's reason phrases, the status
	// lines, and the IANA HTTP Field Name Registry.
	t = append(t, "HTTP/1.0", "HTTP/1.1")
	for code := 100; code < 1000; code++ {
		if p := http.StatusText(code); p != "" {
			t = append(t, p, fmt.Sprintf("HTTP/1.0 %d %s", code, p), fmt.Sprintf("HTTP/1.1 %d %s", code, p))
		}
	}
	t = append(t, ianaFieldNames...)
	// 3. The JSON-RPC 2.0 envelope and its reserved error codes.
	t = append(t, "jsonrpc", "id", "method", "params", "result", "error", "code", "message", "data", `"jsonrpc":"2.0"`,
		"-32700", "Parse error", "-32600", "Invalid Request", "-32601", "Method not found",
		"-32602", "Invalid params", "-32603", "Internal error", "Server error")
	for c := -32099; c <= -32000; c++ {
		t = append(t, strconv.Itoa(c))
	}
	// 4. Server-sent events field names. 5. JSON literals. 6. What os/exec
	// adds on Windows.
	t = append(t, "event", "data", "id", "retry", "true", "false", "null", "SYSTEMROOT")
	slices.Sort(t)
	return slices.Compact(t)
})

// inPublicText reports whether v, as written, occurs in one entry: byte for
// byte, as a substring when v has 8 or more characters, else as a whole
// token by 2(a)'s boundary rule.
func inPublicText(v string) bool {
	if v == "" {
		return false
	}
	long := utf8.RuneCountInString(v) >= pieceLen
	for _, e := range publicText() {
		for i := 0; i < len(e); i++ {
			j := strings.Index(e[i:], v)
			if j < 0 {
				break
			}
			a, b := i+j, i+j+len(v)
			if long || boundary(e[:a], true) && boundary(e[b:], false) {
				return true
			}
			i = a
		}
	}
	return false
}

// boundary reports whether s, the text on one side of a token, ends (before
// it) or starts (after it) at the edge or with a character that is not a
// letter or digit.
func boundary(s string, before bool) bool {
	if s == "" {
		return true
	}
	r, _ := utf8.DecodeRuneInString(s)
	if before {
		r, _ = utf8.DecodeLastRuneInString(s)
	}
	return !unicode.IsLetter(r) && !unicode.IsDigit(r)
}

// startupCollisionLocked is the source of a held value that the matcher, by
// 2(a) with all of section 3, finds whole in one entry taken alone as a
// unit (a check the budget stops counts).
func startupCollisionLocked() (Source, bool) {
	vals := make([]string, 0, len(reg.vals))
	for v := range reg.vals {
		vals = append(vals, v)
	}
	slices.Sort(vals)
	all := newWholeMatcher(vals)
	if all == nil {
		return "", false
	}
	entries := publicText()
	units := make([][]byte, len(entries))
	for i, e := range entries {
		units[i] = []byte(e)
	}
	if all.constClean(true, units...) { // exact: consttable.go
		return "", false
	}
	// Every entry is its own unit; they are checked side by side.
	hit := make([]bool, len(entries))
	var next atomic.Int64
	var wg sync.WaitGroup
	for range min(runtime.GOMAXPROCS(0), 8) {
		wg.Go(func() {
			for i := int(next.Add(1) - 1); i < len(entries); i = int(next.Add(1) - 1) {
				hit[i] = all.check([]byte(entries[i])) != clean
			}
		})
	}
	wg.Wait()
	for i, e := range entries {
		if !hit[i] {
			continue
		}
		for _, v := range vals {
			if m := newWholeMatcher([]string{v}); m != nil && m.check([]byte(e)) != clean {
				return reg.vals[v].src, true
			}
		}
	}
	return "", false
}

// records counts the records still being written; Exit waits for them.
var records sync.WaitGroup

// recordLocked records a value acquired while running that is not held:
// its source, in memory and on stderr (written after the lock is let go,
// and checked like any output; the name is left out when it would match).
func recordLocked(src Source) {
	reg.recorded = append(reg.recorded, src)
	records.Add(1)
	go func() {
		defer records.Done()
		defer emitEnd(emitBegin()) // the barrier, to the end of the write (held.go heldMu)
		msg := recordText(src)
		if withheld([]byte(msg)) {
			msg = recordText("")
		}
		_, _ = os.Stderr.WriteString(msg + "\n")
	}()
}

// refuse ends startup: the refusal, naming src unless that would match,
// then Exit.
func refuse(src Source) {
	e := emitBegin() // the barrier, to the end of the write (held.go heldMu)
	msg := refusalText(src)
	if withheld([]byte(msg)) {
		msg = refusalText("")
	}
	_, _ = os.Stderr.WriteString(msg + "\n")
	emitEnd(e)
	Exit(ExitRefused)
}

// ianaFieldNames is the IANA HTTP Field Name Registry
// (https://www.iana.org/assignments/http-fields/field-names.csv, every
// entry, read 2026-10-30).
var ianaFieldNames = []string{
	"A-IM", "Accept", "Accept-Additions", "Accept-CH", "Accept-Charset", "Accept-Datetime",
	"Accept-Encoding", "Accept-Features", "Accept-Language", "Accept-Patch", "Accept-Post",
	"Accept-Query", "Accept-Ranges", "Accept-Signature", "Access-Control",
	"Access-Control-Allow-Credentials", "Access-Control-Allow-Headers",
	"Access-Control-Allow-Methods", "Access-Control-Allow-Origin", "Access-Control-Expose-Headers",
	"Access-Control-Max-Age", "Access-Control-Request-Headers", "Access-Control-Request-Method",
	"Activate-Storage-Access", "Age", "Allow", "ALPN", "Alt-Svc", "Alt-Used", "Alternates",
	"AMP-Cache-Transform", "Apply-To-Redirect-Ref", "Authentication-Control", "Authentication-Info",
	"Authorization", "Available-Dictionary", "C-Ext", "C-Man", "C-Opt", "C-PEP", "C-PEP-Info",
	"Cache-Control", "Cache-Group-Invalidation", "Cache-Groups", "Cache-Status", "Cal-Managed-ID",
	"CalDAV-Timezones", "Capsule-Protocol", "CDN-Cache-Control", "CDN-Loop", "Cert-Not-After",
	"Cert-Not-Before", "Clear-Site-Data", "Client-Cert", "Client-Cert-Chain", "Close", "CMCD-Object",
	"CMCD-Request", "CMCD-Session", "CMCD-Status", "CMSD-Dynamic", "CMSD-Static",
	"Concealed-Auth-Export", "Configuration-Context", "Connect-UDP-Bind", "Connection",
	"Content-Base", "Content-Digest", "Content-Disposition", "Content-Encoding", "Content-ID",
	"Content-Language", "Content-Length", "Content-Location", "Content-MD5", "Content-Range",
	"Content-Script-Type", "Content-Security-Policy", "Content-Security-Policy-Report-Only",
	"Content-Style-Type", "Content-Type", "Content-Version", "Cookie", "Cookie2",
	"Cross-Origin-Embedder-Policy", "Cross-Origin-Embedder-Policy-Report-Only",
	"Cross-Origin-Opener-Policy", "Cross-Origin-Opener-Policy-Report-Only",
	"Cross-Origin-Resource-Policy", "CTA-Common-Access-Token", "DASL", "Date", "DAV", "Default-Style",
	"Delta-Base", "Deprecation", "Depth", "Derived-From", "Destination", "Detached-JWS",
	"Differential-ID", "Dictionary-ID", "Digest", "DPoP", "DPoP-Nonce", "Early-Data",
	"EDIINT-Features", "ETag", "Expect", "Expect-CT", "Expires", "Ext", "Forwarded", "From",
	"GetProfile", "Hobareg", "Host", "HTTP2-Settings", "If", "If-Match", "If-Modified-Since",
	"If-None-Match", "If-Range", "If-Schedule-Tag-Match", "If-Unmodified-Since", "IM",
	"Include-Referred-Token-Binding-ID", "Incremental", "Isolation", "Keep-Alive", "Label",
	"Last-Event-ID", "Last-Modified", "Link", "Link-Template", "Location", "Lock-Token", "Man",
	"Max-Forwards", "Memento-Datetime", "Meter", "Method-Check", "Method-Check-Expires",
	"MIME-Version", "Negotiate", "NEL", "No-Vary-Search", "OData-EntityId", "OData-Isolation",
	"OData-MaxVersion", "OData-Version", "Opt", "Optional-WWW-Authenticate", "Ordering-Type",
	"Origin", "Origin-Agent-Cluster", "OSCORE", "OSLC-Core-Version", "Overwrite", "P3P", "PEP",
	"PEP-Info", "Permissions-Policy", "PICS-Label", "Ping-From", "Ping-To", "Position", "Pragma",
	"Prefer", "Preference-Applied", "Priority", "ProfileObject", "Protocol", "Protocol-Info",
	"Protocol-Query", "Protocol-Request", "Proxy-Authenticate", "Proxy-Authentication-Info",
	"Proxy-Authorization", "Proxy-Features", "Proxy-Instruction", "Proxy-Public-Address",
	"Proxy-Status", "Public", "Public-Key-Pins", "Public-Key-Pins-Report-Only", "Range",
	"Redirect-Ref", "Referer", "Referer-Root", "Referrer-Policy", "Refresh",
	"Repeatability-Client-ID", "Repeatability-First-Sent", "Repeatability-Request-ID",
	"Repeatability-Result", "Replay-Nonce", "Reporting-Endpoints", "Repr-Digest", "Retry-After",
	"Safe", "Schedule-Reply", "Schedule-Tag", "Sec-Fetch-Dest", "Sec-Fetch-Mode", "Sec-Fetch-Site",
	"Sec-Fetch-Storage-Access", "Sec-Fetch-User", "Sec-GPC", "Sec-Purpose", "Sec-Token-Binding",
	"Sec-WebSocket-Accept", "Sec-WebSocket-Extensions", "Sec-WebSocket-Key", "Sec-WebSocket-Protocol",
	"Sec-WebSocket-Version", "Security-Scheme", "Server", "Server-Timing", "Set-Cookie",
	"Set-Cookie2", "Set-Txn", "SetProfile", "Signature", "Signature-Input", "SLUG", "SoapAction",
	"Status-URI", "Strict-Transport-Security", "Sunset", "Surrogate-Capability", "Surrogate-Control",
	"TCN", "TE", "Timeout", "Timing-Allow-Origin", "Topic", "Traceparent", "Tracestate", "Trailer",
	"Transfer-Encoding", "TTL", "Unencoded-Digest", "Upgrade", "Urgency", "URI", "Use-As-Dictionary",
	"User-Agent", "Variant-Vary", "Vary", "Via", "Want-Content-Digest", "Want-Digest",
	"Want-Repr-Digest", "Want-Unencoded-Digest", "Warning", "WWW-Authenticate",
	"X-Content-Type-Options", "X-Frame-Options", "*",
}
