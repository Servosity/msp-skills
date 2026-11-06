// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
//
// Hand-written (NOT generated): a minimal WS-Management (MS-WSMV) WinRS client.
// It opens a remote shell, runs one command, collects stdout and stderr, and
// closes the shell. Recorded in handfixes.json (winrm-transport).

// Package winrm runs one command on a Windows server over WS-Management.
//
// Security model: the endpoint (host and port) is fixed when the Client is
// built and never changes; requests do not follow redirects and never use a
// proxy, so the credential is only ever sent to that endpoint. HTTPS is the
// default and certificate verification is always on: system roots, a CA
// file, or an exact SHA-256 pin of the server certificate. Plain HTTP exists
// only when the caller sets Endpoint.AllowHTTP, which the CLI exposes as a
// CLI-only flag.
package winrm

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	ntlmssp "github.com/Azure/go-ntlmssp"

	"windows-server-pp-cli/internal/cliutil"
	"windows-server-pp-cli/internal/credkit"
)

// Endpoint is where a server's WinRM listener is and how to trust it.
type Endpoint struct {
	Host          string // DNS name or IP literal, no scheme, port or path
	Port          int    // 5986 for HTTPS (default), 5985 for HTTP
	HTTP          bool   // plain HTTP; refused unless AllowHTTP
	AllowHTTP     bool
	CAFile        string // PEM bundle that replaces the system roots
	CertSHA256    string // exact SHA-256 fingerprint of the server certificate
	TLSServerName string // name to verify when it differs from Host
}

// Auth is the account the shell runs as.
type Auth struct {
	Method   string // "ntlm" (default) or "basic"
	Username string
	Password string
}

// Result is what one command produced.
type Result struct {
	Stdout   []byte
	Stderr   []byte
	ExitCode int
}

// Error kinds the CLI maps to exit codes and messages.
var (
	ErrAuth     = errors.New("winrm: the server rejected the credentials (HTTP 401)")
	ErrRefused  = errors.New("winrm: refused")
	ErrTooLarge = errors.New("winrm: command output exceeded the size limit")
)

// Fault is a WS-Management SOAP fault the server returned.
type Fault struct {
	Code    string
	Message string
}

func (f *Fault) Error() string {
	if f.Code != "" {
		return fmt.Sprintf("winrm fault %s: %s", f.Code, f.Message)
	}
	return "winrm fault: " + f.Message
}

// receiveTimeoutFault is the WSManFault code a Receive returns when the
// command produced nothing within OperationTimeout; the client retries.
const receiveTimeoutFault = "2150858793"

// maxOutput caps stdout plus stderr of one command.
const maxOutput = 64 << 20

// Client talks to one endpoint as one account.
type Client struct {
	url   string
	http  *http.Client
	tr    *http.Transport // closed after each Run; one client serves one script
	auth  Auth
	limit *cliutil.AdaptiveLimiter // paces every post; halves on HTTP 429
}

// ParseFingerprint normalizes a SHA-256 certificate fingerprint ("AB:CD:.."
// or plain hex) to 64 lowercase hex characters. A 40-character value is the
// Windows SHA-1 thumbprint and is refused with a hint.
func ParseFingerprint(s string) (string, error) {
	t := strings.ToLower(strings.NewReplacer(":", "", " ", "", "-", "").Replace(strings.TrimSpace(s)))
	if len(t) == 40 {
		return "", fmt.Errorf("cert_sha256 %q looks like the Windows SHA-1 thumbprint; give the SHA-256 fingerprint of the certificate", s)
	}
	if len(t) != 64 {
		return "", fmt.Errorf("cert_sha256 must be 64 hex characters (SHA-256), got %d", len(t))
	}
	if _, err := hex.DecodeString(t); err != nil {
		return "", fmt.Errorf("cert_sha256 is not hex: %w", err)
	}
	return t, nil
}

// ValidHost reports whether h is a bare DNS name or IP literal.
func ValidHost(h string) bool {
	if h == "" || len(h) > 253 {
		return false
	}
	if ip := net.ParseIP(strings.Trim(h, "[]")); ip != nil {
		return true
	}
	for _, label := range strings.Split(h, ".") {
		if label == "" || len(label) > 63 || label[0] == '-' || label[len(label)-1] == '-' {
			return false
		}
		for _, r := range label {
			if !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '-') {
				return false
			}
		}
	}
	return true
}

// New builds a client. It fails closed on any endpoint it cannot pin or trust.
func New(ep Endpoint, auth Auth, timeout time.Duration) (*Client, error) {
	if !ValidHost(ep.Host) {
		return nil, fmt.Errorf("%w: host %q is not a bare DNS name or IP address", ErrRefused, ep.Host)
	}
	scheme := "https"
	if ep.HTTP {
		if !ep.AllowHTTP {
			return nil, fmt.Errorf("%w: plain HTTP to %s needs the CLI flag --allow-http", ErrRefused, ep.Host)
		}
		scheme = "http"
	}
	port := ep.Port
	if port == 0 {
		port = 5986
		if ep.HTTP {
			port = 5985
		}
	}
	if port < 1 || port > 65535 {
		return nil, fmt.Errorf("%w: port %d out of range", ErrRefused, port)
	}
	method := strings.ToLower(strings.TrimSpace(auth.Method))
	if method == "" {
		method = "ntlm"
	}
	if method != "ntlm" && method != "basic" {
		return nil, fmt.Errorf("%w: auth %q is not ntlm or basic", ErrRefused, auth.Method)
	}
	auth.Method = method
	if ep.HTTP && method == "basic" {
		return nil, fmt.Errorf("%w: Basic auth over plain HTTP sends the password in clear text; use HTTPS, or NTLM", ErrRefused)
	}
	if auth.Username == "" || auth.Password == "" {
		return nil, fmt.Errorf("%w: no username or password for %s", ErrRefused, ep.Host)
	}
	// Every password the leg uses passes here, whatever its source and the
	// auth method (NTLM never puts it in a header heldLeg could see).
	credkit.Hold("config:password", auth.Password)

	tr := &http.Transport{
		Proxy:                 nil, // never route a credential through a proxy
		DialContext:           (&net.Dialer{Timeout: 15 * time.Second, KeepAlive: 30 * time.Second}).DialContext,
		ForceAttemptHTTP2:     false,
		MaxIdleConns:          4,
		IdleConnTimeout:       60 * time.Second,
		TLSHandshakeTimeout:   15 * time.Second,
		ResponseHeaderTimeout: 120 * time.Second,
	}
	if scheme == "https" {
		tc, err := tlsConfig(ep)
		if err != nil {
			return nil, err
		}
		tr.TLSClientConfig = tc
	}
	// heldLeg sits under the NTLM negotiator, so it sees every hop of the
	// handshake as well as the request that carries the payload.
	var rt http.RoundTripper = heldLeg{tr: tr}
	if method == "ntlm" {
		rt = ntlmssp.Negotiator{RoundTripper: heldLeg{tr: tr}}
	}
	hc := &http.Client{
		Transport: rt,
		Timeout:   timeout,
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return fmt.Errorf("%w: the server answered with a redirect; WinRM never follows one", ErrRefused)
		},
	}
	hostPort := net.JoinHostPort(strings.Trim(ep.Host, "[]"), strconv.Itoa(port))
	return &Client{url: scheme + "://" + hostPort + "/wsman", http: hc, tr: tr, auth: auth, limit: cliutil.NewAdaptiveLimiterAuto(20)}, nil
}

func tlsConfig(ep Endpoint) (*tls.Config, error) {
	tc := &tls.Config{MinVersion: tls.VersionTLS12, ServerName: strings.Trim(ep.Host, "[]")}
	if ep.TLSServerName != "" {
		tc.ServerName = ep.TLSServerName
	}
	if ep.CertSHA256 != "" {
		pin, err := ParseFingerprint(ep.CertSHA256)
		if err != nil {
			return nil, err
		}
		// The pin replaces chain and name checks with an exact identity
		// match. VerifyPeerCertificate always runs on a full handshake, so
		// verification is never skipped: no certificate or any other
		// certificate fails the handshake. A resumed session skips
		// VerifyPeerCertificate, so session tickets are off and
		// VerifyConnection checks the same pin on every connection,
		// resumed or not.
		checkPin := func(leaf []byte) error {
			sum := sha256.Sum256(leaf)
			if hex.EncodeToString(sum[:]) != pin {
				return fmt.Errorf("winrm: server certificate SHA-256 %s does not match the pinned cert_sha256", hex.EncodeToString(sum[:]))
			}
			return nil
		}
		tc.InsecureSkipVerify = true // #nosec G402 -- replaced by the exact SHA-256 pin below
		tc.SessionTicketsDisabled = true
		tc.ClientSessionCache = nil
		tc.VerifyPeerCertificate = func(raw [][]byte, _ [][]*x509.Certificate) error {
			if len(raw) == 0 {
				return errors.New("winrm: server sent no certificate")
			}
			return checkPin(raw[0])
		}
		tc.VerifyConnection = func(cs tls.ConnectionState) error {
			if len(cs.PeerCertificates) == 0 {
				return errors.New("winrm: server sent no certificate")
			}
			return checkPin(cs.PeerCertificates[0].Raw)
		}
		return tc, nil
	}
	if ep.CAFile != "" {
		pem, err := os.ReadFile(ep.CAFile) // #nosec G304 -- operator-configured CA bundle path
		if err != nil {
			return nil, fmt.Errorf("reading ca_file: %w", err)
		}
		pool := x509.NewCertPool()
		if !pool.AppendCertsFromPEM(pem) {
			return nil, fmt.Errorf("ca_file %s holds no PEM certificate", ep.CAFile)
		}
		tc.RootCAs = pool
	}
	return tc, nil
}

// Run executes command with args in a fresh shell whose environment holds env,
// and returns its output. stdin is unused. The shell is always deleted.
func (c *Client) Run(ctx context.Context, command string, args []string, env map[string]string) (*Result, error) {
	defer c.tr.CloseIdleConnections() // runs after the shell delete below
	shellID, err := c.createShell(ctx, env)
	if err != nil {
		return nil, err
	}
	defer func() {
		dctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 20*time.Second)
		defer cancel()
		_, _ = c.post(dctx, envelope(c.url, actionDelete, shellID, "", ""))
	}()
	cmdID, err := c.command(ctx, shellID, command, args)
	if err != nil {
		return nil, err
	}
	res, err := c.receive(ctx, shellID, cmdID)
	sctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 20*time.Second)
	defer cancel()
	_, _ = c.post(sctx, envelope(c.url, actionSignal, shellID, "",
		`<rsp:Signal CommandId="`+xmlEscape(cmdID)+`"><rsp:Code>`+signalTerminate+`</rsp:Code></rsp:Signal>`))
	return res, err
}

func (c *Client) createShell(ctx context.Context, env map[string]string) (string, error) {
	var b strings.Builder
	b.WriteString("<rsp:Shell>")
	if len(env) > 0 {
		b.WriteString("<rsp:Environment>")
		for _, k := range sortedKeys(env) {
			b.WriteString(`<rsp:Variable Name="` + xmlEscape(k) + `">` + xmlEscape(env[k]) + `</rsp:Variable>`)
		}
		b.WriteString("</rsp:Environment>")
	}
	b.WriteString("<rsp:InputStreams>stdin</rsp:InputStreams><rsp:OutputStreams>stdout stderr</rsp:OutputStreams></rsp:Shell>")
	opts := `<w:OptionSet><w:Option Name="WINRS_NOPROFILE">TRUE</w:Option><w:Option Name="WINRS_CODEPAGE">65001</w:Option></w:OptionSet>`
	body, err := c.post(ctx, envelope(c.url, actionCreate, "", opts, b.String()))
	if err != nil {
		return "", err
	}
	var env2 soapEnvelope
	if err := xml.Unmarshal(body, &env2); err != nil {
		return "", fmt.Errorf("winrm: decoding create response: %w", err)
	}
	if id := strings.TrimSpace(env2.Body.Shell.ShellID); id != "" {
		return id, nil
	}
	for _, s := range env2.Body.ResourceCreated.Selectors {
		if s.Name == "ShellId" && strings.TrimSpace(s.Value) != "" {
			return strings.TrimSpace(s.Value), nil
		}
	}
	return "", errors.New("winrm: create response carried no ShellId")
}

func (c *Client) command(ctx context.Context, shellID, command string, args []string) (string, error) {
	opts := `<w:OptionSet><w:Option Name="WINRS_CONSOLEMODE_STDIN">TRUE</w:Option><w:Option Name="WINRS_SKIP_CMD_SHELL">TRUE</w:Option></w:OptionSet>`
	body := "<rsp:CommandLine><rsp:Command>" + xmlEscape(command) + "</rsp:Command>"
	if len(args) > 0 {
		body += "<rsp:Arguments>" + xmlEscape(strings.Join(args, " ")) + "</rsp:Arguments>"
	}
	body += "</rsp:CommandLine>"
	resp, err := c.post(ctx, envelope(c.url, actionCommand, shellID, opts, body))
	if err != nil {
		return "", err
	}
	var env soapEnvelope
	if err := xml.Unmarshal(resp, &env); err != nil {
		return "", fmt.Errorf("winrm: decoding command response: %w", err)
	}
	id := strings.TrimSpace(env.Body.CommandResponse.CommandID)
	if id == "" {
		return "", errors.New("winrm: command response carried no CommandId")
	}
	return id, nil
}

func (c *Client) receive(ctx context.Context, shellID, cmdID string) (*Result, error) {
	opts := `<w:OptionSet><w:Option Name="WSMAN_CMDSHELL_OPTION_KEEPALIVE">TRUE</w:Option></w:OptionSet>`
	body := `<rsp:Receive><rsp:DesiredStream CommandId="` + xmlEscape(cmdID) + `">stdout stderr</rsp:DesiredStream></rsp:Receive>`
	res := &Result{}
	for {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		resp, err := c.post(ctx, envelope(c.url, actionReceive, shellID, opts, body))
		if err != nil {
			var f *Fault
			if errors.As(err, &f) && f.Code == receiveTimeoutFault {
				continue
			}
			return nil, err
		}
		var env soapEnvelope
		if err := xml.Unmarshal(resp, &env); err != nil {
			return nil, fmt.Errorf("winrm: decoding receive response: %w", err)
		}
		for _, s := range env.Body.ReceiveResponse.Streams {
			if s.CommandID != "" && s.CommandID != cmdID {
				continue
			}
			raw, err := base64.StdEncoding.DecodeString(strings.TrimSpace(s.Value))
			if err != nil {
				return nil, fmt.Errorf("winrm: decoding %s stream: %w", s.Name, err)
			}
			switch s.Name {
			case "stdout":
				res.Stdout = append(res.Stdout, raw...)
			case "stderr":
				res.Stderr = append(res.Stderr, raw...)
			}
			if len(res.Stdout)+len(res.Stderr) > maxOutput {
				return nil, ErrTooLarge
			}
		}
		st := env.Body.ReceiveResponse.CommandState
		if strings.HasSuffix(st.State, "/CommandState/Done") {
			if code := strings.TrimSpace(st.ExitCode); code != "" {
				n, err := strconv.Atoi(code)
				if err != nil {
					return nil, fmt.Errorf("winrm: exit code %q: %w", code, err)
				}
				res.ExitCode = n
			}
			return res, nil
		}
	}
}

func (c *Client) post(ctx context.Context, payload string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.url, strings.NewReader(payload))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/soap+xml;charset=UTF-8")
	req.SetBasicAuth(c.auth.Username, c.auth.Password)
	if err := c.limit.Wait(ctx); err != nil {
		return nil, err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxOutput))
	if err != nil {
		return nil, err
	}
	switch {
	case resp.StatusCode == http.StatusTooManyRequests:
		c.limit.OnRateLimit()
		return nil, &cliutil.RateLimitError{URL: c.url, Body: "the WinRM listener is throttling requests"}
	case resp.StatusCode == http.StatusUnauthorized && c.auth.Method == "ntlm":
		// WinRM also answers 401 when the logon succeeded but the token is
		// not an administrator's: Windows filters local accounts other than
		// the built-in Administrator on NTLM network logons (remote UAC).
		return nil, fmt.Errorf("%w; with NTLM, a local account other than the built-in Administrator is refused unless LocalAccountTokenFilterPolicy is 1 on the server: use a domain account, or auth = \"basic\" over HTTPS", ErrAuth)
	case resp.StatusCode == http.StatusUnauthorized:
		return nil, ErrAuth
	case resp.StatusCode == http.StatusOK:
		c.limit.OnSuccess()
		return data, nil
	}
	if f := parseFault(data); f != nil {
		return nil, f
	}
	return nil, fmt.Errorf("winrm: HTTP %d from the WinRM listener", resp.StatusCode)
}

func parseFault(data []byte) *Fault {
	var env soapEnvelope
	if xml.Unmarshal(data, &env) != nil || env.Body.Fault == nil {
		return nil
	}
	f := env.Body.Fault
	msg := strings.TrimSpace(f.Detail.WSManFault.Message)
	if msg == "" {
		msg = strings.TrimSpace(f.Reason.Text)
	}
	return &Fault{Code: strings.TrimSpace(f.Detail.WSManFault.Code), Message: strings.Join(strings.Fields(msg), " ")}
}

const (
	resourceCmd     = "http://schemas.microsoft.com/wbem/wsman/1/windows/shell/cmd"
	actionCreate    = "http://schemas.xmlsoap.org/ws/2004/09/transfer/Create"
	actionDelete    = "http://schemas.xmlsoap.org/ws/2004/09/transfer/Delete"
	actionCommand   = "http://schemas.microsoft.com/wbem/wsman/1/windows/shell/Command"
	actionReceive   = "http://schemas.microsoft.com/wbem/wsman/1/windows/shell/Receive"
	actionSignal    = "http://schemas.microsoft.com/wbem/wsman/1/windows/shell/Signal"
	signalTerminate = "http://schemas.microsoft.com/wbem/wsman/1/windows/shell/signal/terminate"
)

func envelope(to, action, shellID, options, body string) string {
	var b strings.Builder
	b.WriteString(`<s:Envelope xmlns:s="http://www.w3.org/2003/05/soap-envelope" xmlns:a="http://schemas.xmlsoap.org/ws/2004/08/addressing" xmlns:w="http://schemas.dmtf.org/wbem/wsman/1/wsman.xsd" xmlns:p="http://schemas.microsoft.com/wbem/wsman/1/wsman.xsd" xmlns:rsp="http://schemas.microsoft.com/wbem/wsman/1/windows/shell"><s:Header>`)
	b.WriteString(`<a:To>` + xmlEscape(to) + `</a:To>`)
	b.WriteString(`<a:ReplyTo><a:Address s:mustUnderstand="true">http://schemas.xmlsoap.org/ws/2004/08/addressing/role/anonymous</a:Address></a:ReplyTo>`)
	b.WriteString(`<w:MaxEnvelopeSize s:mustUnderstand="true">153600</w:MaxEnvelopeSize>`)
	b.WriteString(`<a:MessageID>uuid:` + newUUID() + `</a:MessageID>`)
	b.WriteString(`<w:Locale xml:lang="en-US" s:mustUnderstand="false"/><p:DataLocale xml:lang="en-US" s:mustUnderstand="false"/>`)
	b.WriteString(`<w:OperationTimeout>PT60S</w:OperationTimeout>`)
	b.WriteString(`<w:ResourceURI s:mustUnderstand="true">` + resourceCmd + `</w:ResourceURI>`)
	b.WriteString(`<a:Action s:mustUnderstand="true">` + action + `</a:Action>`)
	if shellID != "" {
		b.WriteString(`<w:SelectorSet><w:Selector Name="ShellId">` + xmlEscape(shellID) + `</w:Selector></w:SelectorSet>`)
	}
	b.WriteString(options)
	b.WriteString(`</s:Header><s:Body>` + body + `</s:Body></s:Envelope>`)
	return b.String()
}

type soapEnvelope struct {
	Body struct {
		Shell struct {
			ShellID string `xml:"ShellId"`
		} `xml:"Shell"`
		ResourceCreated struct {
			Selectors []struct {
				Name  string `xml:"Name,attr"`
				Value string `xml:",chardata"`
			} `xml:"ReferenceParameters>SelectorSet>Selector"`
		} `xml:"ResourceCreated"`
		CommandResponse struct {
			CommandID string `xml:"CommandId"`
		} `xml:"CommandResponse"`
		ReceiveResponse struct {
			Streams []struct {
				Name      string `xml:"Name,attr"`
				CommandID string `xml:"CommandId,attr"`
				Value     string `xml:",chardata"`
			} `xml:"Stream"`
			CommandState struct {
				State    string `xml:"State,attr"`
				ExitCode string `xml:"ExitCode"`
			} `xml:"CommandState"`
		} `xml:"ReceiveResponse"`
		Fault *struct {
			Reason struct {
				Text string `xml:"Text"`
			} `xml:"Reason"`
			Detail struct {
				WSManFault struct {
					Code    string `xml:"Code,attr"`
					Message string `xml:"Message"`
				} `xml:"WSManFault"`
			} `xml:"Detail"`
		} `xml:"Fault"`
	} `xml:"Body"`
}

func xmlEscape(s string) string {
	var b bytes.Buffer
	_ = xml.EscapeText(&b, []byte(s))
	return b.String()
}

func newUUID() string {
	var u [16]byte
	_, _ = rand.Read(u[:])
	u[6] = u[6]&0x0f | 0x40
	u[8] = u[8]&0x3f | 0x80
	h := hex.EncodeToString(u[:])
	return h[0:8] + "-" + h[8:12] + "-" + h[12:16] + "-" + h[16:20] + "-" + h[20:]
}

func sortedKeys(m map[string]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// heldLeg holds, on every hop of the WinRM leg (which is outside
// credkit.Transport, see credkit-sites.json), what contract section 1 says
// a request or response carries as a credential: the request's auth headers
// (Basic parts included) and cookie values, and each Set-Cookie value in the
// response headers and trailers, before the response reaches the program.
type heldLeg struct{ tr *http.Transport }

func (l heldLeg) RoundTrip(req *http.Request) (*http.Response, error) {
	holdRequestHeader(req.Header)
	resp, err := l.tr.RoundTrip(req)
	if resp != nil {
		holdSetCookies(resp.Header)
		if resp.Body != nil {
			resp.Body = &trailerHold{ReadCloser: resp.Body, resp: resp}
		}
	}
	return resp, err
}

func holdRequestHeader(h http.Header) {
	for _, name := range []string{"Authorization", "Proxy-Authorization"} {
		for _, v := range h.Values(name) {
			credkit.Hold("header:"+strings.ToLower(name), v)
		}
	}
	for _, v := range h.Values("Cookie") {
		for _, part := range strings.Split(v, ";") {
			name, val, ok := strings.Cut(strings.TrimSpace(part), "=")
			if !ok {
				val, name = name, "unnamed"
			}
			credkit.Hold("cookie:"+name, val, strings.Trim(val, `"`))
		}
	}
}

// holdSetCookies holds each Set-Cookie value; a line that does not parse is
// held whole, so no reading of it can leave the value out.
func holdSetCookies(h http.Header) {
	for _, line := range h.Values("Set-Cookie") {
		c, err := http.ParseSetCookie(line)
		if err != nil {
			credkit.Hold("cookie:unparsed", line)
			continue
		}
		credkit.Hold("cookie:"+c.Name, c.Value)
	}
}

// trailerHold holds the response trailers' Set-Cookie values once the body
// has been read to its end, when net/http fills them in.
type trailerHold struct {
	io.ReadCloser
	resp *http.Response
}

func (t *trailerHold) Read(p []byte) (int, error) {
	n, err := t.ReadCloser.Read(p)
	if err == io.EOF {
		holdSetCookies(t.resp.Trailer)
	}
	return n, err
}
