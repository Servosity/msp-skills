// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

// Granola's OFFICIAL MCP server (https://mcp.granola.ai) — the connector CONSUMES
// it as an OAuth client to fetch the one thing the public REST API omits: the
// user's raw private/human notes. This is NOT this connector's own MCP surface
// (that is granola-mcp, the agent-native mirror of the CLI). Two different MCPs.
//
// Auth is OAuth 2.1 authorization-code + PKCE (S256) with a loopback redirect;
// tokens live in the macOS Keychain (service granola-cli-mcp), never on disk
// and never in output. macOS-only: /usr/bin/security is the token store.

import (
	"bytes"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

const (
	mcpAuthBase  = "https://mcp-auth.granola.ai"
	mcpURL       = "https://mcp.granola.ai/mcp"
	mcpKCService = "granola-cli-mcp"
	// 127.0.0.1 literal (RFC 8252), NOT "localhost": the listener binds IPv4,
	// and "localhost" could resolve to ::1 in the browser and miss the callback.
	mcpRedirect   = "http://127.0.0.1:8799/callback"
	mcpLoopback   = "127.0.0.1:8799"
	mcpProtoVer   = "2025-06-18"
	mcpClientName = "granola-cli"
)

// Keychain accounts under service granola-cli-mcp.
const (
	kcAcctAccess   = "MCP_ACCESS_TOKEN"
	kcAcctRefresh  = "MCP_REFRESH_TOKEN"
	kcAcctClientID = "MCP_CLIENT_ID"
	kcAcctExpiry   = "MCP_TOKEN_EXPIRY" // unix seconds
)

// maxMCPBody caps every response we read so a hostile/broken server can't OOM
// us. get_meetings digests run a few KB; 16MB is generous headroom.
const maxMCPBody = 16 << 20

// readCappedMCPBody reads at most maxMCPBody bytes and REJECTS a larger body
// rather than silently truncating it (a plain io.LimitReader would hand back a
// corrupt, quietly-shortened JSON-RPC envelope or SSE stream). The read error
// is surfaced, never swallowed. Mirrors client.readCappedBody.
func readCappedMCPBody(r io.Reader) ([]byte, error) {
	data, err := io.ReadAll(io.LimitReader(r, maxMCPBody+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > maxMCPBody {
		return nil, fmt.Errorf("mcp response body exceeds %d-byte limit", int64(maxMCPBody))
	}
	return data, nil
}

// mcpHTTPClient refuses to follow redirects: a 307/308 from the token endpoint
// would otherwise replay the form body (refresh token / PKCE verifier) to a
// server-chosen Location. Callers treat any 3xx as an error.
var mcpHTTPClient = &http.Client{
	Timeout: 45 * time.Second,
	CheckRedirect: func(*http.Request, []*http.Request) error {
		return http.ErrUseLastResponse
	},
}

// --- Keychain (macOS) ---------------------------------------------------------

// The token store is security(1). Every call runs it in interactive mode
// (`security -i`) and hands it the command line on STDIN, never on argv:
//
//   - the secret no longer rides on the process argument list, where any
//     same-user `ps` watcher could read it while the call ran (the residual the
//     argv form had to accept);
//   - the exec argv is a fixed literal pair, so nothing caller-supplied ever
//     reaches exec.Command.
//
// security(1)'s interactive parser honors double quotes with backslash
// escapes, so kcQuote wraps every operand and escapes `"` and `\`. A control
// character (newline above all) would end the command line early and let the
// remainder run as a second command, so kcQuote refuses one outright. A real
// OAuth token is printable ASCII (RFC 6749 VSCHAR) and never trips it.
//
// kcKeychainPath, when non-empty, names an explicit keychain file instead of
// the user's default search list. Production never sets it; the opt-in
// integration test points it at a throwaway keychain.
var kcKeychainPath string

func kcQuote(v string) (string, error) {
	for _, r := range v {
		if r < 0x20 || r == 0x7f {
			return "", errors.New("keychain operand contains a control character")
		}
	}
	return `"` + strings.NewReplacer(`\`, `\\`, `"`, `\"`).Replace(v) + `"`, nil
}

// kcLine renders one security(1) command line from a subcommand and its
// operands. Every operand is quoted, flags included (the parser strips the
// quotes), so a value that happens to begin with "-" or contain a space can
// never be misread. The explicit keychain, when set, is appended last.
func kcLine(sub string, args ...string) (string, error) {
	parts := []string{sub}
	for _, a := range args {
		q, err := kcQuote(a)
		if err != nil {
			return "", err
		}
		parts = append(parts, q)
	}
	if kcKeychainPath != "" {
		q, err := kcQuote(kcKeychainPath)
		if err != nil {
			return "", err
		}
		parts = append(parts, q)
	}
	line := strings.Join(parts, " ") + "\n"
	// security(1) reads interactive input with a 4096-byte line buffer. A
	// longer line is split at the buffer boundary and the remainder runs as a
	// second command, so the rendered line (newline included) is capped well
	// under it. Real values are far shorter: an OAuth access token is about a
	// kilobyte.
	if len(line) > kcMaxLine {
		return "", fmt.Errorf("keychain command line is %d bytes, over the %d-byte limit", len(line), kcMaxLine)
	}
	return line, nil
}

// kcMaxLine bounds one security(1) interactive command line, newline included.
const kcMaxLine = 4000

// kcRun executes one command line through `security -i`. The exit status of
// the command propagates (44 = errSecItemNotFound), so kcNotFound keeps
// working unchanged.
func kcRun(line string, capture bool) ([]byte, error) {
	cmd := exec.Command("/usr/bin/security", "-i")
	cmd.Stdin = strings.NewReader(line)
	if capture {
		return cmd.Output()
	}
	return nil, cmd.Run()
}

// kcSet stores a secret. The value never touches disk, argv, or CLI output.
func kcSet(account, value string) error {
	line, err := kcLine("add-generic-password", "-U", "-a", account, "-s", mcpKCService, "-w", value)
	if err != nil {
		return err
	}
	_, err = kcRun(line, false)
	return err
}

func kcGet(account string) (string, error) {
	line, err := kcLine("find-generic-password", "-a", account, "-s", mcpKCService, "-w")
	if err != nil {
		return "", err
	}
	out, err := kcRun(line, true)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

func kcDelete(account string) error {
	line, err := kcLine("delete-generic-password", "-a", account, "-s", mcpKCService)
	if err != nil {
		return err
	}
	_, err = kcRun(line, false)
	return err
}

// kcNotFound reports whether a security(1) call failed only because the item
// was absent (exit 44 = errSecItemNotFound) — an expected no-op, not a failure.
func kcNotFound(err error) bool {
	var ee *exec.ExitError
	if errors.As(err, &ee) {
		return ee.ExitCode() == 44
	}
	return false
}

// --- OAuth --------------------------------------------------------------------

func mcpJSON(method, u string, payload any, headers map[string]string) (int, http.Header, []byte, error) {
	var body io.Reader
	h := map[string]string{"Accept": "application/json"}
	if payload != nil {
		b, err := json.Marshal(payload)
		if err != nil {
			return 0, nil, nil, err
		}
		body = bytes.NewReader(b)
		h["Content-Type"] = "application/json"
	}
	for k, v := range headers {
		h[k] = v
	}
	req, err := http.NewRequest(method, u, body)
	if err != nil {
		return 0, nil, nil, err
	}
	for k, v := range h {
		req.Header.Set(k, v)
	}
	resp, err := mcpHTTPClient.Do(req)
	if err != nil {
		return 0, nil, nil, err
	}
	defer resp.Body.Close()
	data, rerr := readCappedMCPBody(resp.Body)
	if rerr != nil {
		return resp.StatusCode, resp.Header, nil, fmt.Errorf("reading mcp response: %w", rerr)
	}
	return resp.StatusCode, resp.Header, data, nil
}

func mcpForm(u string, form url.Values) (int, []byte, error) {
	req, err := http.NewRequest("POST", u, strings.NewReader(form.Encode()))
	if err != nil {
		return 0, nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")
	resp, err := mcpHTTPClient.Do(req)
	if err != nil {
		return 0, nil, err
	}
	defer resp.Body.Close()
	data, rerr := readCappedMCPBody(resp.Body)
	if rerr != nil {
		return resp.StatusCode, nil, fmt.Errorf("reading mcp response: %w", rerr)
	}
	return resp.StatusCode, data, nil
}

// mcpRegister does dynamic client registration and returns a client_id. The
// authorization server only permits the authorization_code + refresh_token
// grants and requires redirect_uris (device_code is advertised but not
// registerable), so we register a native public client.
func mcpRegister() (string, error) {
	st, _, b, err := mcpJSON("POST", mcpAuthBase+"/oauth2/register", map[string]any{
		"client_name":                mcpClientName,
		"grant_types":                []string{"authorization_code", "refresh_token"},
		"redirect_uris":              []string{mcpRedirect},
		"token_endpoint_auth_method": "none",
		"application_type":           "native",
		"scope":                      "mcp",
	}, nil)
	if err != nil {
		return "", err
	}
	// Never echo the response body: OAuth error/registration payloads can carry
	// tokens or secrets. Status code only.
	if st/100 != 2 {
		return "", fmt.Errorf("client registration failed (HTTP %d)", st)
	}
	var reg struct {
		ClientID string `json:"client_id"`
	}
	if err := json.Unmarshal(b, &reg); err != nil || reg.ClientID == "" {
		return "", fmt.Errorf("client registration returned no client_id")
	}
	return reg.ClientID, nil
}

// pkcePair and randToken fail hard on CSPRNG error rather than silently using
// low-entropy material (which would weaken PKCE/state).
func pkcePair() (verifier, challenge string, err error) {
	raw := make([]byte, 40)
	if _, err = io.ReadFull(rand.Reader, raw); err != nil {
		return "", "", fmt.Errorf("generating PKCE verifier: %w", err)
	}
	verifier = base64.RawURLEncoding.EncodeToString(raw)
	sum := sha256.Sum256([]byte(verifier))
	challenge = base64.RawURLEncoding.EncodeToString(sum[:])
	return verifier, challenge, nil
}

func randToken(n int) (string, error) {
	raw := make([]byte, n)
	if _, err := io.ReadFull(rand.Reader, raw); err != nil {
		return "", fmt.Errorf("generating random token: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(raw), nil
}

func mcpAuthorizeURL(clientID, challenge, state string) string {
	q := url.Values{
		"response_type":         {"code"},
		"client_id":             {clientID},
		"redirect_uri":          {mcpRedirect},
		"scope":                 {"mcp"},
		"code_challenge":        {challenge},
		"code_challenge_method": {"S256"},
		"state":                 {state},
	}
	return mcpAuthBase + "/oauth2/authorize?" + q.Encode()
}

type mcpToken struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	ExpiresIn    int    `json:"expires_in"`
	Scope        string `json:"scope"`
	TokenType    string `json:"token_type"`
}

func mcpExchange(clientID, code, verifier string) (*mcpToken, error) {
	st, b, err := mcpForm(mcpAuthBase+"/oauth2/token", url.Values{
		"grant_type":    {"authorization_code"},
		"code":          {code},
		"redirect_uri":  {mcpRedirect},
		"client_id":     {clientID},
		"code_verifier": {verifier},
	})
	if err != nil {
		return nil, err
	}
	if st/100 != 2 {
		return nil, fmt.Errorf("token exchange failed (HTTP %d)", st)
	}
	var t mcpToken
	if err := json.Unmarshal(b, &t); err != nil || t.AccessToken == "" {
		return nil, fmt.Errorf("token exchange returned no access_token")
	}
	return &t, nil
}

func mcpRefresh(clientID, refresh string) (*mcpToken, error) {
	st, b, err := mcpForm(mcpAuthBase+"/oauth2/token", url.Values{
		"grant_type":    {"refresh_token"},
		"refresh_token": {refresh},
		"client_id":     {clientID},
	})
	if err != nil {
		return nil, err
	}
	if st/100 != 2 {
		return nil, fmt.Errorf("token refresh failed (HTTP %d)", st)
	}
	var t mcpToken
	if err := json.Unmarshal(b, &t); err != nil || t.AccessToken == "" {
		return nil, fmt.Errorf("token refresh returned no access_token")
	}
	return &t, nil
}

// storeMCPToken persists a token bundle in the Keychain. A refresh response may
// omit refresh_token (reuse the prior one), so only overwrite when present.
// kcSetFn is kcSet, indirected so the persistence order can be tested.
var kcSetFn = kcSet

func storeMCPToken(clientID string, t *mcpToken) error {
	// Hand-wired (handfixes.json: mcp-refresh-rotation-persistence): Granola
	// rotates the refresh token on every use, so the NEW refresh token is the
	// one value that cannot be recovered once lost. It is written first, with
	// one retry, and a failure on any item no longer stops the others from
	// being attempted; every failure is returned together.
	set := func(account, value string) error {
		err := kcSetFn(account, value)
		if err != nil {
			err = kcSetFn(account, value)
		}
		return err
	}
	var errs []error
	if t.RefreshToken != "" {
		if err := set(kcAcctRefresh, t.RefreshToken); err != nil {
			errs = append(errs, fmt.Errorf("%w: %v", errRefreshNotSaved, err))
		}
	}
	accessSaved := true
	if err := set(kcAcctAccess, t.AccessToken); err != nil {
		accessSaved = false
		errs = append(errs, fmt.Errorf("access token: %w", err))
	}
	if clientID != "" {
		if err := set(kcAcctClientID, clientID); err != nil {
			errs = append(errs, fmt.Errorf("client id: %w", err))
		}
	}
	// The expiry describes the STORED access token, so it only advances when
	// that token was saved. Otherwise it is set to the past, which makes the
	// next run refresh (with the refresh token saved above) instead of
	// trusting a stale access token as fresh.
	exp := time.Now().Add(time.Duration(t.ExpiresIn) * time.Second)
	if !accessSaved {
		exp = time.Unix(1, 0)
	}
	if err := set(kcAcctExpiry, strconv.FormatInt(exp.Unix(), 10)); err != nil {
		errs = append(errs, fmt.Errorf("expiry: %w", err))
	}
	return errors.Join(errs...)
}

// errRefreshNotSaved marks a storeMCPToken failure that lost the rotated
// refresh token: the old one is already retired, so the session cannot be
// renewed without a new login.
var errRefreshNotSaved = errors.New("refresh token not saved")

// mcpAccessToken returns a usable access token, refreshing via the stored
// refresh_token when the current one is within 60s of expiry. Returns
// errMCPNotConnected (checked with mcpNotConnected) when no tokens are stored,
// and a distinct error when the token is expired and cannot be refreshed —
// callers must not treat an expired/unrefreshable token as connected.
func mcpAccessToken() (string, error) {
	access, err := kcGet(kcAcctAccess)
	if err != nil || access == "" {
		return "", errMCPNotConnected
	}
	expStr, expErr := kcGet(kcAcctExpiry)
	if expErr != nil {
		return access, nil // unknown expiry: try as-is; a 401 prompts re-login
	}
	unix, perr := strconv.ParseInt(expStr, 10, 64)
	if perr != nil {
		return access, nil
	}
	if time.Now().Before(time.Unix(unix, 0).Add(-60 * time.Second)) {
		return access, nil // still valid
	}
	// Expired — a refresh is required; surface failure rather than hand back a
	// known-dead token.
	refresh, _ := kcGet(kcAcctRefresh)
	clientID, _ := kcGet(kcAcctClientID)
	if refresh == "" || clientID == "" {
		return "", fmt.Errorf("granola mcp token expired and cannot refresh — re-run 'granola-cli mcp-auth login'")
	}
	t, rerr := mcpRefresh(clientID, refresh)
	if rerr != nil {
		return "", fmt.Errorf("granola mcp token refresh failed — re-run 'granola-cli mcp-auth login'")
	}
	if serr := storeMCPToken(clientID, t); serr != nil {
		if errors.Is(serr, errRefreshNotSaved) {
			// The rotated refresh token is gone and the old one is retired:
			// the session cannot be renewed. Fail loudly rather than carry on
			// as if all were well.
			return "", fmt.Errorf("granola mcp token refreshed but could not be saved to the Keychain (%v) — re-run 'granola-cli mcp-auth login'", serr)
		}
		// The refresh token is safe; only the access token or its metadata
		// failed to save, and the expiry was set so the next run refreshes.
		stderr("warning: part of the refreshed Granola MCP token could not be saved to the Keychain (%v); the next run will refresh again.", serr)
	}
	return t.AccessToken, nil
}

var errMCPNotConnected = fmt.Errorf("granola mcp not connected: run 'granola-cli mcp-auth login'")

func mcpNotConnected(err error) bool { return err == errMCPNotConnected }

// --- MCP JSON-RPC -------------------------------------------------------------

// mcpCall issues one JSON-RPC request. Granola's MCP replies with SSE
// (text/event-stream) for some methods, so parse the last data: frame when the
// content type says so. Returns the parsed envelope and the session id.
func mcpCall(access, method string, params any, sessionID string) (map[string]any, string, error) {
	h := map[string]string{
		"Authorization": "Bearer " + access,
		"Accept":        "application/json, text/event-stream",
	}
	if sessionID != "" {
		h["Mcp-Session-Id"] = sessionID
	}
	st, hdr, b, err := mcpJSON("POST", mcpURL, map[string]any{
		"jsonrpc": "2.0", "id": 1, "method": method, "params": params,
	}, h)
	if err != nil {
		return nil, sessionID, err
	}
	if st == http.StatusUnauthorized {
		return nil, sessionID, fmt.Errorf("mcp %s: unauthorized (token expired — re-run mcp-auth login)", method)
	}
	sid := hdr.Get("Mcp-Session-Id")
	if sid == "" {
		sid = sessionID
	}
	obj, perr := parseMCPResult(method, st, hdr.Get("Content-Type"), b)
	if perr != nil {
		return nil, sid, perr
	}
	return obj, sid, nil
}

// parseMCPResult validates an MCP HTTP response and returns the JSON-RPC
// envelope. It rejects any non-2xx status BEFORE trusting the body (so a 3xx
// whose body happens to parse can't masquerade as success), fails on an
// unparseable 2xx body, and surfaces a top-level JSON-RPC error by NUMERIC code
// only — never the raw message/body, which can carry tokens or note content.
func parseMCPResult(method string, st int, contentType string, b []byte) (map[string]any, error) {
	if st/100 != 2 {
		return nil, fmt.Errorf("mcp %s failed (HTTP %d)", method, st)
	}
	var obj map[string]any
	if strings.Contains(contentType, "text/event-stream") {
		for _, ln := range strings.Split(string(b), "\n") {
			ln = strings.TrimSpace(ln)
			if after, ok := strings.CutPrefix(ln, "data:"); ok {
				var m map[string]any
				if json.Unmarshal([]byte(strings.TrimSpace(after)), &m) == nil {
					obj = m
				}
			}
		}
	} else {
		_ = json.Unmarshal(b, &obj)
	}
	if obj == nil {
		return nil, fmt.Errorf("mcp %s: unparseable response", method)
	}
	if em, ok := obj["error"].(map[string]any); ok {
		if code, ok := em["code"].(float64); ok {
			return nil, fmt.Errorf("mcp %s protocol error (code %d)", method, int(code))
		}
		return nil, fmt.Errorf("mcp %s protocol error", method)
	}
	return obj, nil
}

// mcpSession runs initialize + notifications/initialized and returns a live
// session id ready for tools/list or tools/call.
func mcpSession(access string) (string, error) {
	_, sid, err := mcpCall(access, "initialize", map[string]any{
		"protocolVersion": mcpProtoVer,
		"capabilities":    map[string]any{},
		"clientInfo":      map[string]any{"name": mcpClientName, "version": version},
	}, "")
	if err != nil {
		return "", err
	}
	_, _, _ = mcpCall(access, "notifications/initialized", map[string]any{}, sid)
	return sid, nil
}
