// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"regexp"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"granola-pp-cli/internal/cliutil"
)

// newMCPAuthCmd manages OAuth with Granola's OFFICIAL MCP server, the source of
// the raw private/human notes the public REST API omits. Distinct from this
// connector's own MCP surface (granola-mcp). macOS-only (Keychain-backed).
func newMCPAuthCmd(flags *rootFlags) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "mcp-auth",
		Short: "Connect Granola's official MCP (for private/human notes REST omits)",
		Long: `Connect this connector to Granola's OFFICIAL MCP server (https://mcp.granola.ai).

This is OPTIONAL. The GRANOLA_API_KEY (public REST) powers everything —
sync, meetings, transcripts, summaries, folders. Granola's MCP adds only the
one thing REST omits: your raw private/human notes (memo, extract, notes show).

This is NOT this CLI's own MCP surface (granola-mcp, the agent-native tool
mirror). Two different MCPs — see the README.

Tokens are stored in the macOS Keychain (service granola-cli-mcp), never on
disk and never printed.`,
	}
	cmd.AddCommand(newMCPAuthLoginCmd(flags))
	cmd.AddCommand(newMCPAuthStatusCmd(flags))
	cmd.AddCommand(newMCPAuthVerifyCmd(flags))
	cmd.AddCommand(newMCPAuthLogoutCmd(flags))
	return cmd
}

// newMCPAuthVerifyCmd proves the connection returns private/human notes end to
// end: list_meetings → a real Granola UUID → get_meetings([uuid]). Prints only
// structural signals (which section labels are present) — never note content.
func newMCPAuthVerifyCmd(flags *rootFlags) *cobra.Command {
	return &cobra.Command{
		Use:   "verify",
		Short: "Confirm private/human notes are reachable (list_meetings → get_meetings)",
		Example: strings.Trim(`
  granola-cli mcp-auth verify
  granola-cli mcp-auth verify --json`, "\n"),
		Annotations: map[string]string{
			"mcp:read-only": "true",
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			w := cmd.OutOrStdout()
			// Live MCP calls; short-circuit under verify/dogfood sandboxes.
			if cliutil.IsVerifyEnv() || isDogfoodEnv() {
				fmt.Fprintln(w, "verify: would probe Granola MCP get_meetings (no-op)")
				return nil
			}
			access, err := mcpAccessToken()
			if err != nil {
				if mcpNotConnected(err) {
					return authErr(err)
				}
				return apiErr(err)
			}
			sid, err := mcpSession(access)
			if err != nil {
				return apiErr(err)
			}
			// Source a native Granola UUID from list_meetings (get_meetings
			// validates meeting_ids as UUIDs; REST's not_* ids are rejected).
			listText, listErr := mcpToolCall(access, sid, "list_meetings", map[string]any{})
			if listErr {
				return apiErr(fmt.Errorf("list_meetings returned an error"))
			}
			uuid := firstGranolaUUID(listText)
			if uuid == "" {
				if flags.asJSON {
					return printJSONFiltered(w, map[string]any{"connected": true, "sampled": false}, flags)
				}
				fmt.Fprintln(w, green("Connected. list_meetings returned data, but no meeting to sample."))
				return nil
			}
			gmText, isErr := mcpToolCall(access, sid, "get_meetings", map[string]any{"meeting_ids": []string{uuid}})
			if isErr {
				// Never echo the tool body: it can carry note content.
				return apiErr(fmt.Errorf("get_meetings returned an error"))
			}
			hits := sectionLabelsPresent(gmText)
			human := contains(hits, "Notes") || contains(hits, "Human") || contains(hits, "Private")
			if flags.asJSON {
				return printJSONFiltered(w, map[string]any{
					"connected":          true,
					"sampled":            true,
					"payload_len":        len(gmText),
					"sections_present":   hits,
					"human_notes_likely": human,
				}, flags)
			}
			fmt.Fprintln(w, green("get_meetings OK — Granola MCP is returning meeting data."))
			fmt.Fprintf(w, "  payload: %d bytes; sections: %s\n", len(gmText), strings.Join(hits, ", "))
			if human {
				fmt.Fprintf(w, "  %s human/private notes reachable → memo, extract, notes show can enrich.\n", green("✓"))
			} else {
				fmt.Fprintln(w, "  note: no notes section detected in the sampled meeting.")
			}
			return nil
		},
	}
}

// mcpToolCall issues a tools/call and returns the concatenated text content and
// whether the tool reported isError.
func mcpToolCall(access, sid, name string, arguments map[string]any) (string, bool) {
	obj, _, err := mcpCall(access, "tools/call", map[string]any{"name": name, "arguments": arguments}, sid)
	if err != nil || obj == nil {
		return "", true // transport/protocol failure; caller reports generically
	}
	result, _ := obj["result"].(map[string]any)
	isErr, _ := result["isError"].(bool)
	var b strings.Builder
	if content, ok := result["content"].([]any); ok {
		for _, c := range content {
			if cm, ok := c.(map[string]any); ok {
				if t, ok := cm["text"].(string); ok {
					b.WriteString(t)
				}
			}
		}
	}
	return b.String(), isErr
}

var granolaUUIDRe = regexp.MustCompile(`[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}`)

func firstGranolaUUID(s string) string { return granolaUUIDRe.FindString(s) }

// sectionLabelsPresent reports which meeting-section labels the payload carries,
// so verify can prove human notes without echoing their content.
func sectionLabelsPresent(payload string) []string {
	var hits []string
	for _, k := range []string{"Notes", "Private", "Human", "Transcript", "Summary"} {
		if strings.Contains(payload, k) {
			hits = append(hits, k)
		}
	}
	return hits
}

func contains(ss []string, want string) bool {
	for _, s := range ss {
		if s == want {
			return true
		}
	}
	return false
}

func newMCPAuthLoginCmd(flags *rootFlags) *cobra.Command {
	var noOpen bool
	var timeout time.Duration
	cmd := &cobra.Command{
		Use:   "login",
		Short: "Authorize Granola's official MCP via browser (OAuth + PKCE)",
		Example: "  granola-cli mcp-auth login\n" +
			"  granola-cli mcp-auth login --no-open   # print the URL instead of opening it",
		Annotations: map[string]string{
			// Human-in-the-loop browser approval; never an agent tool.
			"mcp:hidden": "true",
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			w := cmd.OutOrStdout()
			if dryRunOK(flags) {
				return nil
			}
			// Verify/dogfood sandboxes: no browser, no listener, no network.
			// This interactive command (loopback wait) would otherwise hang the
			// dogfood runner's per-command timeout.
			if cliutil.IsVerifyEnv() || isDogfoodEnv() {
				fmt.Fprintln(w, "verify: would start Granola MCP OAuth (no-op)")
				return nil
			}
			if runtime.GOOS != "darwin" {
				return ioErr(fmt.Errorf("mcp-auth uses the macOS Keychain; not supported on %s", runtime.GOOS))
			}

			clientID, err := mcpRegister()
			if err != nil {
				return apiErr(fmt.Errorf("registering MCP client: %w", err))
			}
			verifier, challenge, err := pkcePair()
			if err != nil {
				return ioErr(err)
			}
			state, err := randToken(16)
			if err != nil {
				return ioErr(err)
			}
			authURL := mcpAuthorizeURL(clientID, challenge, state)

			// Loopback listener catches the redirect with the auth code. Timeouts
			// keep a stray local connection from wedging the listener.
			ln, err := net.Listen("tcp", mcpLoopback)
			if err != nil {
				return ioErr(fmt.Errorf("binding loopback %s (another login running?): %w", mcpLoopback, err))
			}
			codeCh := make(chan string, 1)
			errCh := make(chan error, 1)
			srv := &http.Server{
				Handler:           loopbackHandler(state, codeCh, errCh),
				ReadHeaderTimeout: 5 * time.Second,
				ReadTimeout:       10 * time.Second,
				IdleTimeout:       30 * time.Second,
				MaxHeaderBytes:    1 << 16,
			}
			go func() { _ = srv.Serve(ln) }()
			defer func() {
				ctx, cancel := context.WithTimeout(context.Background(), time.Second)
				defer cancel()
				_ = srv.Shutdown(ctx)
			}()

			// Don't print the authorize URL (it carries state + challenge — a
			// local watcher could replay it) unless the user opted out of the
			// browser open and needs to copy it themselves.
			if noOpen {
				fmt.Fprintln(w, "Authorize granola-cli with Granola — open this URL, approve, and I'll continue:")
				fmt.Fprintln(w, "  "+authURL)
			} else if oerr := openSetupURL(authURL); oerr != nil {
				fmt.Fprintln(w, "Could not open a browser. Re-run with --no-open to get the URL to open manually.")
				return ioErr(fmt.Errorf("launching browser: %w", oerr))
			} else {
				fmt.Fprintln(w, "Opened Granola authorization in your browser. Waiting for you to approve…")
			}

			var code string
			select {
			case code = <-codeCh:
			case e := <-errCh:
				return apiErr(fmt.Errorf("authorization: %w", e))
			case <-time.After(timeout):
				return ioErr(fmt.Errorf("timed out after %s waiting for browser approval; re-run when ready", timeout))
			}

			tok, err := mcpExchange(clientID, code, verifier)
			if err != nil {
				return apiErr(err)
			}
			if err := storeMCPToken(clientID, tok); err != nil {
				return ioErr(fmt.Errorf("storing tokens in Keychain: %w", err))
			}
			fmt.Fprintln(w, green("Connected. Tokens stored in macOS Keychain (granola-cli-mcp)."))
			// Meetings already in the local store are not revisited by sync
			// until Granola changes them, so say how to backfill their notes
			// (handfixes.json: mcp-auth-login-backfill-hint).
			fmt.Fprintln(w, "Private notes are added as meetings are enriched. For meetings you already synced, run: granola-cli enrich   (or enrich --full for your whole library)")

			// Live proof: open a session and list the tools. No note content is
			// printed — only tool names + whether the human-notes tool is present.
			sid, err := mcpSession(tok.AccessToken)
			if err != nil {
				fmt.Fprintf(w, "  (connected, but tools/list probe failed: %v)\n", err)
				return nil
			}
			obj, _, err := mcpCall(tok.AccessToken, "tools/list", map[string]any{}, sid)
			if err != nil {
				fmt.Fprintf(w, "  (connected, but tools/list probe failed: %v)\n", err)
				return nil
			}
			names := toolNames(obj)
			if len(names) > 0 {
				fmt.Fprintf(w, "  MCP tools available: %s\n", strings.Join(names, ", "))
			}
			hasNotes := false
			for _, n := range names {
				if n == "get_meetings" {
					hasNotes = true
				}
			}
			if hasNotes {
				fmt.Fprintln(w, "  get_meetings present → private/human notes reachable for memo/extract/notes show.")
			} else {
				fmt.Fprintln(w, "  note: get_meetings not advertised; human-notes surface may differ.")
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&noOpen, "no-open", false, "Print the authorization URL instead of opening a browser")
	cmd.Flags().DurationVar(&timeout, "timeout", 5*time.Minute, "How long to wait for browser approval")
	return cmd
}

// loopbackHandler returns the /callback handler that validates state and hands
// the auth code back over codeCh (or an error over errCh).
func loopbackHandler(wantState string, codeCh chan<- string, errCh chan<- error) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/callback", func(rw http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		// Validate state FIRST. A forged/mismatched callback (any local process
		// or a web page poking our loopback) is answered 400 and IGNORED — it
		// must never abort the real flow, or it's a trivial local DoS.
		if q.Get("state") != wantState {
			rw.WriteHeader(http.StatusBadRequest)
			return
		}
		if e := q.Get("error"); e != "" {
			writeCallbackPage(rw, "Authorization failed. You can close this tab.")
			// Only the bounded, sanitized OAuth error code — never
			// error_description (attacker-controlled; may carry control chars).
			errCh <- fmt.Errorf("authorization denied (%s)", sanitizeCode(e))
			return
		}
		code := q.Get("code")
		if code == "" {
			writeCallbackPage(rw, "No code returned. You can close this tab.")
			errCh <- fmt.Errorf("no authorization code in callback")
			return
		}
		writeCallbackPage(rw, "granola-cli is connected. You can close this tab.")
		codeCh <- code
	})
	return mux
}

// sanitizeCode reduces an OAuth error code to a short [A-Za-z_] token so a
// hostile redirect can't inject terminal control sequences into our output.
func sanitizeCode(s string) string {
	var b strings.Builder
	for _, r := range s {
		if r == '_' || (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') {
			b.WriteRune(r)
		}
		if b.Len() >= 40 {
			break
		}
	}
	if b.Len() == 0 {
		return "unknown"
	}
	return b.String()
}

func writeCallbackPage(rw http.ResponseWriter, msg string) {
	rw.Header().Set("Content-Type", "text/html")
	rw.WriteHeader(http.StatusOK)
	_, _ = rw.Write([]byte("<!doctype html><meta charset=utf-8><title>granola-cli</title>" +
		"<body style=\"font:16px system-ui;margin:3rem\"><h2>" + msg + "</h2></body>"))
}

func toolNames(obj map[string]any) []string {
	var names []string
	result, _ := obj["result"].(map[string]any)
	tools, _ := result["tools"].([]any)
	for _, t := range tools {
		if tm, ok := t.(map[string]any); ok {
			if n, ok := tm["name"].(string); ok {
				names = append(names, n)
			}
		}
	}
	sort.Strings(names)
	return names
}

func newMCPAuthStatusCmd(flags *rootFlags) *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Show whether Granola's official MCP is connected",
		Example: strings.Trim(`
  granola-cli mcp-auth status
  granola-cli mcp-auth status --json`, "\n"),
		Annotations: map[string]string{
			"mcp:read-only": "true",
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			w := cmd.OutOrStdout()
			// PASSIVE inspection only — this command is mcp:read-only, so it must
			// not hit the network or rotate/persist tokens. Check Keychain
			// presence + the locally-stored expiry; never refresh here (login
			// and the enrich commands own refresh).
			access, kerr := kcGet(kcAcctAccess)
			present := kerr == nil && access != ""
			expired := false
			if present {
				if expStr, e := kcGet(kcAcctExpiry); e == nil {
					if unix, e2 := strconv.ParseInt(expStr, 10, 64); e2 == nil {
						expired = !time.Now().Before(time.Unix(unix, 0))
					}
				}
			}
			connected := present && !expired
			if flags.asJSON {
				return printJSONFiltered(w, map[string]any{
					"connected": connected,
					"expired":   expired,
					"service":   mcpKCService,
					"server":    mcpURL,
				}, flags)
			}
			if !connected {
				if expired {
					fmt.Fprintln(w, red("Granola MCP: token expired"))
					fmt.Fprintln(w, "  Reconnect with: granola-cli mcp-auth login")
					return nil
				}
				fmt.Fprintln(w, red("Granola MCP: not connected"))
				fmt.Fprintln(w, "  Optional — only needed for private/human notes.")
				fmt.Fprintln(w, "  Connect with: granola-cli mcp-auth login")
				return nil
			}
			fmt.Fprintln(w, green("Granola MCP: connected"))
			fmt.Fprintf(w, "  Server:   %s\n", mcpURL)
			fmt.Fprintf(w, "  Keychain: %s\n", mcpKCService)
			return nil
		},
	}
}

func newMCPAuthLogoutCmd(flags *rootFlags) *cobra.Command {
	return &cobra.Command{
		Use:   "logout",
		Short: "Clear stored Granola MCP tokens from the Keychain",
		Annotations: map[string]string{
			"mcp:hidden": "true",
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			// Report honestly: a Keychain deletion failure must not be swallowed
			// (a token that survives while we claim "cleared" is a real risk).
			// "item not found" is the expected no-op and is not a failure.
			var failed []string
			for _, acct := range []string{kcAcctAccess, kcAcctRefresh, kcAcctClientID, kcAcctExpiry} {
				if err := kcDelete(acct); err != nil && !kcNotFound(err) {
					failed = append(failed, acct)
				}
			}
			if len(failed) > 0 {
				if flags.asJSON {
					_ = printJSONFiltered(cmd.OutOrStdout(), map[string]any{"cleared": false, "failed": failed}, flags)
				}
				return ioErr(fmt.Errorf("could not clear %d Keychain item(s): %s", len(failed), strings.Join(failed, ", ")))
			}
			if flags.asJSON {
				return printJSONFiltered(cmd.OutOrStdout(), map[string]any{"cleared": true}, flags)
			}
			fmt.Fprintln(cmd.OutOrStdout(), "Granola MCP tokens cleared.")
			return nil
		},
	}
}
