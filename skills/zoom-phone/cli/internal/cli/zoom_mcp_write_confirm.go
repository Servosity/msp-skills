// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync/atomic"

	"github.com/spf13/cobra"

	"zoom-phone-pp-cli/internal/client"
	"zoom-phone-pp-cli/internal/cliutil"
)

// zoom-phone hand-fix: zoom-phone-mcp-write-confirm.
//
// An MCP tool call may change Zoom only with confirm: true. The server's own
// zoom-phone_execute tool previews until confirm is boolean true; mirrored CLI
// commands run as child processes marked ZOOM_PHONE_MCP_CHILD=1, where:
//   - offboard --apply and fleet --allow-writes return their preview unless
//     --confirm (the tool's confirm argument) is set;
//   - every request that could change Zoom (any method but GET, HEAD or
//     OPTIONS, except the OAuth token mint) is answered 428 without reaching
//     the network unless the invoked command was given that --confirm, so no
//     other mirrored command can write without it either.
//
// A terminal run never sets ZOOM_PHONE_MCP_CHILD, so the CLI is unchanged.
var zoomMCPConfirmCommands = []string{"offboard", "fleet", "import"}

const (
	zoomMCPConfirmFlag           = "confirm"
	zoomMCPConfirmFlagAnnotation = "zoom:mcp-confirm"
	zoomOAuthMintURL             = "https://zoom.us/oauth/token"
)

// zoomMCPWriteConfirmed is set from --confirm when a confirm-capable command
// starts; every other command leaves it false.
var zoomMCPWriteConfirmed atomic.Bool

func init() {
	registerNovelCommand(func(root *cobra.Command, _ *rootFlags) {
		for _, path := range zoomMCPConfirmCommands {
			cmd := zoomFindExact(root, path)
			if cmd == nil || cmd.RunE == nil || cmd.Flags().Lookup(zoomMCPConfirmFlag) != nil {
				continue
			}
			zoomAddMCPConfirmFlag(cmd)
			inner := cmd.RunE
			cmd.RunE = func(c *cobra.Command, args []string) error {
				zoomMCPWriteConfirmed.Store(zoomMCPConfirmGiven(c))
				return inner(c, args)
			}
		}
	})
	registerClientHook(func(c *client.Client) error {
		if c.HTTPClient == nil {
			return nil
		}
		base := c.HTTPClient.Transport
		if base == nil {
			base = cliutil.OutboundGuard(nil) // credkit-wiring
		}
		if _, ok := base.(zoomMCPWriteGate); !ok {
			c.HTTPClient.Transport = zoomMCPWriteGate{base: base, c: c}
		}
		return nil
	})
}

const zoomMCPConfirmUsage = "MCP tool calls only: send the Zoom changes this call makes (without it an MCP call returns the preview)"

// zoomMCPConfirmBare is what pflag hands --confirm's value when the flag is
// given bare. The MCP server emits a bare --confirm only for the JSON boolean
// true; every other value (the string "1" or "true", a number, an array
// element) arrives as --confirm=<value>. A process argument can never contain
// NUL, so no --confirm=<value> can equal this sentinel.
const zoomMCPConfirmBare = "\x00zoom-mcp-confirm"

// zoomMCPConfirmValue is --confirm in an MCP child process: true only for the
// bare flag, so confirmation is the JSON boolean true and nothing coerced
// from another type, exactly as zoom-phone_execute accepts it.
type zoomMCPConfirmValue struct{ confirmed bool }

func (v *zoomMCPConfirmValue) Set(s string) error {
	v.confirmed = s == zoomMCPConfirmBare
	return nil
}

func (v *zoomMCPConfirmValue) String() string { return strconv.FormatBool(v.confirmed) }

// Type keeps the MCP tool schema boolean.
func (v *zoomMCPConfirmValue) Type() string { return "bool" }

// zoomAddMCPConfirmFlag adds --confirm to cmd. The MCP server builds tool
// schemas in its own process (a plain bool flag, so the schema is boolean);
// a mirrored command's child process (ZOOM_PHONE_MCP_CHILD=1) gets the strict
// value that only the bare flag sets. There it is hidden from help, which
// would otherwise print the sentinel.
func zoomAddMCPConfirmFlag(cmd *cobra.Command) {
	if cliutil.IsMCPChildEnv() {
		f := cmd.Flags().VarPF(&zoomMCPConfirmValue{}, zoomMCPConfirmFlag, "", zoomMCPConfirmUsage)
		f.NoOptDefVal = zoomMCPConfirmBare
		f.DefValue = "false"
		f.Hidden = true
	} else {
		cmd.Flags().Bool(zoomMCPConfirmFlag, false, zoomMCPConfirmUsage)
	}
	_ = cmd.Flags().SetAnnotation(zoomMCPConfirmFlag, zoomMCPConfirmFlagAnnotation, []string{"true"})
}

// zoomMCPConfirmGiven reports whether cmd carries this file's --confirm and
// it is true. A same-named flag from anywhere else never counts.
func zoomMCPConfirmGiven(cmd *cobra.Command) bool {
	f := cmd.Flags().Lookup(zoomMCPConfirmFlag)
	if f == nil || len(f.Annotations[zoomMCPConfirmFlagAnnotation]) == 0 {
		return false
	}
	v, err := cmd.Flags().GetBool(zoomMCPConfirmFlag)
	return err == nil && v
}

// zoomMCPPreviewOnly reports whether cmd, run from an MCP tool call, may only
// preview the Zoom changes it would make.
func zoomMCPPreviewOnly(cmd *cobra.Command) bool {
	return cliutil.IsMCPChildEnv() && !zoomMCPConfirmGiven(cmd)
}

const zoomMCPPreviewNote = "preview only: an MCP tool call changes Zoom only with confirm: true; review this preview, then call again with confirm: true"

type zoomMCPWriteGate struct {
	base http.RoundTripper
	c    *client.Client
}

func (t zoomMCPWriteGate) RoundTrip(req *http.Request) (*http.Response, error) {
	if !cliutil.IsMCPChildEnv() || zoomMCPWriteConfirmed.Load() || zoomReadMethod(req.Method) || t.tokenMint(req) {
		return t.base.RoundTrip(req)
	}
	if req.Body != nil {
		_ = req.Body.Close()
	}
	// A 4xx is final for the client (no retry), and the request never left.
	body, _ := json.Marshal(map[string]string{
		"code":    "mcp_confirm_required",
		"message": fmt.Sprintf("refused %s %s from an MCP tool call: changing Zoom needs confirm: true", req.Method, req.URL.Path),
	})
	return &http.Response{
		Status:        "428 Precondition Required",
		StatusCode:    http.StatusPreconditionRequired,
		Proto:         "HTTP/1.1",
		ProtoMajor:    1,
		ProtoMinor:    1,
		Header:        http.Header{"Content-Type": []string{"application/json"}},
		Body:          io.NopCloser(bytes.NewReader(body)),
		ContentLength: int64(len(body)),
		Request:       req,
	}, nil
}

func zoomReadMethod(method string) bool {
	switch strings.ToUpper(strings.TrimSpace(method)) {
	case "", http.MethodGet, http.MethodHead, http.MethodOptions:
		return true
	}
	return false
}

// tokenMint is the client's own OAuth token POST: exactly its token URL.
func (t zoomMCPWriteGate) tokenMint(req *http.Request) bool {
	if req.Method != http.MethodPost || req.URL == nil {
		return false
	}
	tokenURL := zoomOAuthMintURL
	if t.c != nil && t.c.Config != nil && strings.TrimSpace(t.c.Config.TokenURL) != "" {
		tokenURL = strings.TrimSpace(t.c.Config.TokenURL)
	}
	want, err := url.Parse(tokenURL)
	if err != nil {
		return false
	}
	return strings.EqualFold(req.URL.Scheme, want.Scheme) && strings.EqualFold(req.URL.Host, want.Host) &&
		req.URL.User == nil && req.URL.Path == want.Path
}
