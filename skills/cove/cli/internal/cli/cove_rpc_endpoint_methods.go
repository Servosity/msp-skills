// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
//
// Hand-written (NOT generated), cove only: pins each raw generated endpoint
// command to the one JSON-RPC method its spec names (coverpc.EndpointMethod).
// The --method default is set to that method, so a reprint that prints an
// empty default (as press 4.34.0 did for audit, devices stats, storage stats
// and storage nodes) still sends it, and a --method naming any other method is
// refused before anything is sent. The load-bearing write guard is in the
// generated client (internal/client/cove_rpc_method_guard.go); this pin keeps
// each command to the method it documents. Registered from init through the
// generated root's novel-command hooks, so no generated file is edited.
// Hand-fix code: cove-rpc-read-method-guard.

package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"cove-pp-cli/internal/coverpc"
)

func init() {
	registerNovelCommand(func(root *cobra.Command, _ *rootFlags) {
		pinCoveEndpointMethods(root)
	})
}

func pinCoveEndpointMethods(cmd *cobra.Command) {
	for _, sub := range cmd.Commands() {
		pinCoveEndpointMethods(sub)
	}
	method, ok := coverpc.EndpointMethod(cmd.Annotations["pp:endpoint"])
	if !ok {
		return
	}
	flag := cmd.Flags().Lookup("method")
	if flag == nil {
		return
	}
	if flag.Value.String() != method {
		// Value.Set leaves Changed false, so a bare invocation still prints help.
		_ = flag.Value.Set(method)
	}
	flag.DefValue = method
	if cmd.RunE == nil {
		return
	}
	next := cmd.RunE
	cmd.RunE = func(c *cobra.Command, args []string) error {
		if f := c.Flags().Lookup("method"); f != nil && f.Changed && f.Value.String() != method {
			return usageErr(fmt.Errorf("%s always calls the JSON-RPC method %s; --method %q is refused. To call another method use `cove-cli call <Method>` (a method other than Get... or Enumerate... also needs --yes)", c.CommandPath(), method, f.Value.String()))
		}
		return next(c, args)
	}
}
