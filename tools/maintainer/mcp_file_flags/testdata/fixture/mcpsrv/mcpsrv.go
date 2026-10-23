// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

// Package mcpsrv is the analyzer's self-test fixture for in-process MCP tool
// handlers that touch the disk through an os.Root. Every handler named
// handleWant* must be reported as letting the MCP caller name a host path,
// and every handler named handleComp* only as choosing an entry inside a
// directory the operator chose.
package mcpsrv

import (
	"context"
	"io"
	"os"
	"path/filepath"

	"github.com/mark3labs/mcp-go/mcp"
)

// uploadRootEnv is the operator's opt-in upload root, as press 4.34.0 names
// <SLUG>_MCP_UPLOAD_ROOT.
const uploadRootEnv = "FIXTURE_MCP_UPLOAD_ROOT"

// resolveUpload has the shape of press 4.34.0's resolveMCPUploadFromRoot.
func resolveUpload(root, raw string) ([]byte, error) {
	r, err := os.OpenRoot(root)
	if err != nil {
		return nil, err
	}
	defer r.Close()
	rel := raw
	if filepath.IsAbs(raw) {
		rr, err := filepath.Rel(root, raw)
		if err != nil {
			return nil, err
		}
		rel = rr
	}
	f, err := r.Open(filepath.ToSlash(rel))
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return io.ReadAll(f)
}

// handleCompUploadRoot is the 4.34.0 template's case: the root is the
// operator's environment setting, the caller names a file inside it.
func handleCompUploadRoot(_ context.Context, req mcp.CallToolRequest) error {
	_, err := resolveUpload(os.Getenv(uploadRootEnv), req.GetString("file", ""))
	return err
}

// handleCompConstRoot opens a constant root in place.
func handleCompConstRoot(_ context.Context, req mcp.CallToolRequest) error {
	r, err := os.OpenRoot(".")
	if err != nil {
		return err
	}
	defer r.Close()
	_, err = r.ReadFile(req.GetString("file", ""))
	return err
}

// handleWantRootFromArgument opens the root at a path the MCP caller sent.
func handleWantRootFromArgument(_ context.Context, req mcp.CallToolRequest) error {
	r, err := os.OpenRoot(req.GetString("dir", ""))
	if err != nil {
		return err
	}
	defer r.Close()
	_, err = r.Open("report.csv")
	return err
}

// handleWantRootHelperFromArgument passes a caller-chosen root to the same
// helper the template uses: the helper's root is steered at this call site.
func handleWantRootHelperFromArgument(_ context.Context, req mcp.CallToolRequest) error {
	_, err := resolveUpload(req.GetString("dir", ""), "report.csv")
	return err
}

// handleWantOpenBesideRoot has a confined root, and a plain os.Open of the
// caller's value next to it.
func handleWantOpenBesideRoot(_ context.Context, req mcp.CallToolRequest) error {
	if _, err := resolveUpload(os.Getenv(uploadRootEnv), req.GetString("file", "")); err != nil {
		return err
	}
	f, err := os.Open(req.GetString("file", ""))
	if err != nil {
		return err
	}
	return f.Close()
}

// Handlers are registered by reference so none is dead code.
var Handlers = []func(context.Context, mcp.CallToolRequest) error{
	handleCompUploadRoot, handleCompConstRoot, handleWantRootFromArgument,
	handleWantRootHelperFromArgument, handleWantOpenBesideRoot,
}
