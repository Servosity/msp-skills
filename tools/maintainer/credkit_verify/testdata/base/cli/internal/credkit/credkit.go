// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

// Package credkit stands in for the shared credential kit: its exported API,
// none of its behavior. The checker skips this package and resolves calls to it.
package credkit

import (
	"database/sql/driver"
	"io"
	"net/http"
	"os"
	"os/exec"

	"github.com/mark3labs/mcp-go/server"
)

type Source = string
type Identifier struct {
	Source Source
	Reason string
}
type Origin struct {
	Source  Source
	Allowed []string
}
type BaseURL struct{ ConfigKey, Env, Default string }
type Declarations struct {
	SecretEnv         []string
	Files             func(args []string) []string
	ConfigFields      []string
	StoredRecords     []string
	CredentialFlags   []string
	StdinFlags        []string
	CredentialHeaders []string
	CredentialFields  []string
	ResponseFields    []string
	MCPArguments      []string
	BaseURL           BaseURL
	Origins           []Origin
	Identifiers       []Identifier
}

func Start(d Declarations)                                                {}
func Exit(code int)                                                       { os.Exit(code) }
func Hold(source Source, values ...string)                                {}
func Transport(inner http.RoundTripper) http.RoundTripper                 { return inner }
func WriteFile(name string, data []byte, perm os.FileMode) error          { return nil }
func OpenFile(name string, flag int, perm os.FileMode) (io.Writer, error) { return io.Discard, nil }
func SQLDriver(inner driver.Driver) driver.Driver                         { return inner }
func StartCommand(cmd *exec.Cmd) error                                    { return nil }
func RunCommand(cmd *exec.Cmd) error                                      { return nil }
func MCPTools(next server.ToolHandlerFunc) server.ToolHandlerFunc         { return next }
func MCPIn(r io.Reader) io.Reader                                         { return r }
func MCPOut() io.Writer                                                   { return os.Stdout }
func MCPHTTP(h http.Handler) http.Handler                                 { return h }
func FlagValues(args []string, flag string) []string                      { return nil }
