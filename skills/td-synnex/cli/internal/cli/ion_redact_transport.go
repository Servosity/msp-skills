// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-written: every ION response is redacted before the CLI or MCP server sees it.

package cli

import (
	"bytes"
	"compress/flate"
	"compress/gzip"
	"compress/zlib"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"td-synnex-pp-cli/internal/client"
	"td-synnex-pp-cli/internal/cliutil"
)

// ION returns credentials in ordinary responses: customers carry a user with a
// generated password, order items can carry Microsoft tenant passwords as
// name/value attributes, provisioning returns tenant admin credentials, and a
// customer's cloud provider link holds azureCustomerInfo (Partner Center JSON
// with admin credentials). Redacting at the transport means the response
// cache, the synced store, printed output, --deliver sinks and MCP tool
// results only ever see judged bytes. The OAuth token exchange is exempt:
// auth mint and auth check need its tokens and never print them unasked.
func init() {
	// A --dry-run request body is judged by the same redactor before it is
	// printed: an order can carry a tenant admin password the operator typed.
	client.DryRunBodyFilter = redactSecretsJSON
	registerClientHook(func(c *client.Client) error {
		if c == nil || c.HTTPClient == nil {
			return nil
		}
		if _, done := c.HTTPClient.Transport.(redactingTransport); done {
			return nil
		}
		base := c.HTTPClient.Transport
		if base == nil {
			base = http.DefaultTransport
		}
		hc := *c.HTTPClient
		hc.Transport = redactingTransport{base: base}
		c.HTTPClient = &hc
		return nil
	})
}

// maxRedactBodyBytes caps how much of one response is read for judging; a
// larger body is refused rather than passed through unjudged.
const maxRedactBodyBytes = 64 << 20

type redactingTransport struct{ base http.RoundTripper }

// transportKindError reads as its kind alone; Unwrap keeps errors.Is/As.
type transportKindError struct {
	kind string
	err  error
}

func (e transportKindError) Error() string { return e.kind }
func (e transportKindError) Unwrap() error { return e.err }

// ionHostAllowed is where an ION token may travel: ion.tdsynnex.com, a
// loopback test server on this machine, or any host when TD_SYNNEX_BASE_URL
// names a test or proxy endpoint for this run. A remote base_url read from
// config.toml or a --config file is not trusted on its own, for the access
// token as for the refresh token (adversarial round 13, P1: API calls followed
// a file base_url that auth mint refused). Loopback stays allowed so doctor
// and test stubs work: reaching it already takes a process on this machine.
func ionHostAllowed(host string) bool {
	return strings.EqualFold(host, "ion.tdsynnex.com") || isLoopbackHost(host) || cliutil.EnvOverride("TD_SYNNEX_BASE_URL") != ""
}

func (t redactingTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if req.URL != nil && strings.EqualFold(req.URL.Scheme, "http") && !isLoopbackHost(req.URL.Hostname()) {
		if req.Body != nil {
			req.Body.Close()
		}
		return nil, fmt.Errorf("refusing plain HTTP to %s: ION tokens travel only over https (only a loopback test server may use http)", req.URL.Host)
	}
	if req.URL != nil && !ionHostAllowed(req.URL.Hostname()) {
		if req.Body != nil {
			req.Body.Close()
		}
		return nil, fmt.Errorf("refusing to send the ION token to %s: base_url points away from ion.tdsynnex.com and TD_SYNNEX_BASE_URL is not set for this run", req.URL.Host)
	}
	resp, err := t.base.RoundTrip(req)
	if err != nil {
		// net/http quotes server bytes (a malformed header line) in its
		// error; only the kind reaches errors and retry warnings. The
		// original unwraps for timeout and retry checks (Codex round 15, P1).
		return nil, transportKindError{kind: ionTransportErrorKind(err), err: err}
	}
	// Only a request postIONForm marked is exempt: a URL path alone could be
	// reached by a redirect from an ordinary API call.
	if err != nil || resp == nil || (req.Context().Value(ionOAuthExchange{}) != nil && isIONTokenExchange(req.URL)) {
		return resp, err
	}
	// Headers reach errors and retry warnings (a redirect Location with
	// ?password=...), so they are judged like body text.
	for k, vs := range resp.Header {
		for i, v := range vs {
			if isSecretKey(k) || hasSecretText(v, false) {
				vs[i] = redacted
			}
		}
	}
	if resp.Body == nil {
		return resp, nil
	}
	body, rerr := readJudgeable(resp)
	resp.Body.Close()
	if rerr != nil {
		// A body-read error can quote server bytes (a malformed chunk trailer
		// such as password=...), so only its kind is reported (Codex round 19, P1).
		kind := ionTransportErrorKind(rerr)
		for _, known := range []error{errRedactTooLarge, errRedactEncoding} {
			if errors.Is(rerr, known) {
				kind = known.Error()
			}
		}
		return nil, transportKindError{kind: "reading ION response for redaction: " + kind, err: rerr}
	}
	out := []byte(redactSecretsJSON(body))
	resp.Body = io.NopCloser(bytes.NewReader(out))
	resp.ContentLength = int64(len(out))
	resp.Header.Set("Content-Length", strconv.Itoa(len(out)))
	resp.Header.Del("Content-Encoding")
	resp.Uncompressed = true
	return resp, nil
}

// isLoopbackHost reports localhost and loopback IP literals, the only hosts a
// test or mock server may serve plain HTTP from.
func isLoopbackHost(h string) bool {
	if strings.EqualFold(h, "localhost") {
		return true
	}
	ip := net.ParseIP(h)
	return ip != nil && ip.IsLoopback()
}

// isIONTokenExchange reports the two OAuth endpoints auth mint and auth check
// call (https://<host>/oauth/token and /oauth/validateAccess).
// ionOAuthExchange marks the context of the OAuth form requests auth mint and
// auth check send; their responses carry the tokens and pass unredacted.
type ionOAuthExchange struct{}

func isIONTokenExchange(u *url.URL) bool {
	if u == nil {
		return false
	}
	p := strings.TrimRight(u.Path, "/")
	return p == "/oauth/token" || p == "/oauth/validateAccess"
}

// readJudgeable reads the body and undoes any Content-Encoding net/http left
// in place, so the redactor never judges compressed bytes.
func readJudgeable(resp *http.Response) ([]byte, error) {
	body, err := readCapped(resp.Body)
	if err != nil {
		return nil, err
	}
	enc := strings.ToLower(strings.TrimSpace(resp.Header.Get("Content-Encoding")))
	switch enc {
	case "", "identity":
		return body, nil
	case "gzip", "x-gzip":
		zr, err := gzip.NewReader(bytes.NewReader(body))
		if err != nil {
			return nil, err
		}
		defer zr.Close()
		return readCapped(zr)
	case "deflate":
		if zr, err := zlib.NewReader(bytes.NewReader(body)); err == nil {
			defer zr.Close()
			if out, err := readCapped(zr); err == nil {
				return out, nil
			}
		}
		fr := flate.NewReader(bytes.NewReader(body))
		defer fr.Close()
		return readCapped(fr)
	}
	return nil, errRedactEncoding
}

var (
	errRedactTooLarge = errors.New("response exceeds the redaction size limit")
	errRedactEncoding = errors.New("unsupported Content-Encoding")
)

func readCapped(r io.Reader) ([]byte, error) {
	buf, err := io.ReadAll(io.LimitReader(r, maxRedactBodyBytes+1))
	if err != nil {
		return nil, err
	}
	if len(buf) > maxRedactBodyBytes {
		return nil, errRedactTooLarge
	}
	return buf, nil
}
