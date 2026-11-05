// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

// Hand-written (netsapiens; handfixes.json doctor-verdict-tracks-reality):
// makes `doctor` report a verdict that tracks reality. Fleet pattern from
// skills/ninjaone/cli/internal/cli/doctor_verdict.go (issue #282), adapted to
// NetSapiens: every provider runs its own platform host, and the shipped
// server default (ns-api.com) is only the public sandbox.
//
// Before this file, doctor called any HTTP answer at "/" reachable and never
// probed the credential ("present, not verified"), so a working install, a
// rejected key and a base URL aimed at a web UI all read the same; and an
// operator who never set NETSAPIENS_SERVER was told the sandbox was reachable.

package cli

import (
	"context"
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"netsapiens-pp-cli/internal/client"
	"netsapiens-pp-cli/internal/config"
)

// doctorBaseURLEnv overrides the whole API root; doctorServerEnv fills the
// host in the shipped https://{server}/ns-api/v2. Remedies name both.
const (
	doctorBaseURLEnv = "NETSAPIENS_BASE_URL"
	doctorServerEnv  = "NETSAPIENS_SERVER"
)

// doctorProbeEndpoint is the credential probe: GET /apikeys/~ answers with
// the key making the request (spec.json "Read API Key info on your API Key"),
// so it works for every scope (reseller, office manager, read-only) and can
// only succeed when the credential itself is accepted.
const doctorProbeEndpoint = "apikeys.my"

// doctorServerIsShippedDefault reports an install still on the shipped server
// default: the base URL carries {server} and nothing (environment, config
// template_vars) set it, so requests would go to the public sandbox, not the
// operator's platform. An operator who sets NETSAPIENS_SERVER=ns-api.com on
// purpose is probed normally.
func doctorServerIsShippedDefault(cfg *config.Config) bool {
	return cfg != nil && strings.Contains(cfg.BaseURL, "{server}") && cfg.ServerIsShippedDefault()
}

// doctorPlaceholderVerdict is the api row for an install on a placeholder or
// the shipped sandbox default. It names both variables that fix it.
func doctorPlaceholderVerdict(cfg *config.Config) string {
	if doctorServerIsShippedDefault(cfg) {
		return "not configured (base_url is a placeholder): " + doctorServerEnv + " is unset, so requests would go to the public sandbox ns-api.com; set " + doctorServerEnv + " to your platform host, or " + doctorBaseURLEnv + " to the full API root (https://<host>/ns-api/v2)"
	}
	return "not configured (base_url is a placeholder); set " + doctorServerEnv + " to your platform host, or " + doctorBaseURLEnv + " to the full API root (https://<host>/ns-api/v2)"
}

// doctorReadProbe returns the API path to probe and the command that runs it:
// apikeys.my when the tree has it, else the first argument-free GET leaf.
func doctorReadProbe(root *cobra.Command) (apiPath, cmdPath string) {
	var fallbackAPI, fallbackCmd string
	var walk func(*cobra.Command, []string)
	walk = func(cmd *cobra.Command, path []string) {
		for _, child := range cmd.Commands() {
			if apiPath != "" {
				return
			}
			childPath := append(append([]string{}, path...), child.Name())
			if p := doctorProbeableLeaf(child); p != "" {
				if child.Annotations["pp:endpoint"] == doctorProbeEndpoint {
					apiPath, cmdPath = p, strings.Join(childPath, " ")
					return
				}
				if fallbackAPI == "" {
					fallbackAPI, fallbackCmd = p, strings.Join(childPath, " ")
				}
			}
			walk(child, childPath)
		}
	}
	if root != nil {
		walk(root, nil)
	}
	if apiPath == "" {
		apiPath, cmdPath = fallbackAPI, fallbackCmd
	}
	return apiPath, cmdPath
}

// doctorProbeableLeaf returns the leaf's API path when it is a GET with no
// path parameters and no required arguments, or "".
func doctorProbeableLeaf(cmd *cobra.Command) string {
	if cmd == nil || cmd.Hidden || cmd.HasSubCommands() || !cmd.Runnable() {
		return ""
	}
	if cmd.Annotations["pp:endpoint"] == "" || !strings.EqualFold(cmd.Annotations["pp:method"], "GET") {
		return ""
	}
	path := cmd.Annotations["pp:path"]
	if path == "" || !strings.HasPrefix(path, "/") || strings.ContainsAny(path, "{}") || strings.ContainsAny(cmd.Use, "<[") {
		return ""
	}
	if cmd.Args != nil && cmd.Args(cmd, []string{}) != nil {
		return ""
	}
	return path
}

// doctorProbeCredentials issues one authenticated read (never cached: ProbeGet
// goes through the client's do) and writes the credentials verdict and, when
// the probe shows the api row was optimistic, a corrected api verdict.
func doctorProbeCredentials(ctx context.Context, c *client.Client, root *cobra.Command, bin string, report map[string]any) {
	if c == nil || c.Config == nil || c.Config.AuthHeader() == "" {
		report["credentials"] = "ERROR not verified: the client has no credential to send, so any response would say nothing about authentication."
		return
	}
	apiPath, cmdPath := doctorReadProbe(root)
	if apiPath == "" {
		report["credentials"] = "WARN not verified: this API exposes no argument-free GET endpoint to probe. Run any read command to confirm the credential works end-to-end."
		report["api"] = "WARN reachable at the transport level only - no argument-free GET endpoint exists to confirm this is the vendor's API or that the credential works."
		return
	}
	status, err := c.ProbeGet(ctx, apiPath)
	switch {
	case err == nil:
		report["credentials"] = fmt.Sprintf("valid (verified with GET %s)", apiPath)
	case status == 401:
		report["credentials"] = fmt.Sprintf("ERROR rejected (HTTP 401 from %s) - the credential is invalid, expired, or for a different platform.", apiPath)
	case status == 403:
		report["credentials"] = fmt.Sprintf("WARN scope-limited (HTTP 403 from %s) - permission denied for this endpoint, so the credential was not confirmed. Run a read command your role allows to check it.", apiPath)
	case status == 404:
		report["credentials"] = fmt.Sprintf("ERROR not verified (HTTP 404 from %s) - the endpoint does not exist at this base_url, so the credential was never checked.", apiPath)
		report["api"] = fmt.Sprintf("ERROR the host answered but %s is not there - base_url is not the NetSapiens API root. Set %s to your platform host, or %s to https://<host>/ns-api/v2.", apiPath, doctorServerEnv, doctorBaseURLEnv)
	case status >= 500:
		report["credentials"] = fmt.Sprintf("WARN not verified (HTTP %d from %s) - the API returned a server error, so the credential was never checked.", status, apiPath)
	case status > 0:
		report["credentials"] = fmt.Sprintf("WARN not verified (HTTP %d from %s). Run `%s %s` to see the full response.", status, apiPath, bin, cmdPath)
	default:
		report["credentials"] = fmt.Sprintf("ERROR not verified: %s", err)
	}
}
