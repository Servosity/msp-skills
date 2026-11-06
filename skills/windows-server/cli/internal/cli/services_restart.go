// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
//
// Hand-written (NOT generated): `services restart` and `services start`, the
// only commands that change a server. Recorded in handfixes.json
// (service-actions).
// pp:data-source live

package cli

import (
	"encoding/json"
	"errors"
	"fmt"

	"github.com/spf13/cobra"

	"windows-server-pp-cli/internal/winsrv"
)

// Each constructor names its Use literally so dogfood's source scan finds it.
func newNovelServicesRestartCmd(flags *rootFlags) *cobra.Command {
	return newServiceActionCmd(flags, &cobra.Command{Use: "restart <service-name>"}, "restart")
}

func newServicesStartCmd(flags *rootFlags) *cobra.Command {
	return newServiceActionCmd(flags, &cobra.Command{Use: "start <service-name>"}, "start")
}

func newServiceActionCmd(flags *rootFlags, cmd *cobra.Command, action string) *cobra.Command {
	var confirm bool
	short := "Restart a misbehaving service, with a preview first and the service state before and after."
	long := `Restarts one running service on one server (--server, or the default server).

Without --yes (CLI) or confirm: true (MCP) it only previews: the service's
state, start mode and running dependents, and whether the restart is allowed.
Refused, always: WinRM itself and every service WinRM depends on (the
restart would cut this session), a service with running dependents (no
-Force is ever used), and a service that is not running (use services start).`
	if action == "start" {
		short = "Start a stopped automatic service, with a preview first and the service state before and after."
		long = `Starts one stopped automatic-start service on one server (--server, or the
default server): the fix for a row in services down.

Without --yes (CLI) or confirm: true (MCP) it only previews: the service's
state and start mode, and whether the start is allowed. Refused, always: a
service that is not stopped, a manual or disabled service, and WinRM itself
and the services it depends on.`
	}
	cmd.Short, cmd.Long = short, long
	cmd.Example = fmt.Sprintf("  windows-server-cli services %s Spooler --server fs01 --agent\n  windows-server-cli services %s Spooler --server fs01 --yes", action, action)
	cmd.Args = cobra.ExactArgs(1)
	// Dogfood fixtures: W32Time runs on every Windows Server. A restart runs
	// for real only on a run with --allow-destructive (the test server); a
	// start of a running service previews its refusal (exit 2, nothing changed).
	cmd.Annotations = map[string]string{"mcp:read-only": "false", "pp:data-source": "live", "pp:typed-exit-codes": "0,2"}
	if action == "restart" {
		cmd.Annotations["pp:live-happy-path"] = "true"
		cmd.Annotations["pp:happy-args"] = "service-name=W32Time;--yes"
	} else {
		cmd.Annotations["pp:preview-happy-path"] = "true"
		cmd.Annotations["pp:happy-args"] = "service-name=W32Time"
	}
	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		name := args[0]
		params := map[string]any{"name": name, "action": action}
		if _, err := mustLookup("service_plan").Validate(params); err != nil {
			return usageErr(err)
		}
		if dryRunOK(flags) {
			target := "the default server"
			if winsrvServer != "" {
				target = "server " + winsrvServer
			}
			return writeDryRun(cmd.OutOrStdout(), flags, fmt.Sprintf("services %s %s on %s after a preview", action, name, target))
		}
		profiles, credErrs, err := winsrvServers(flags, false)
		if err != nil {
			return err
		}
		p := profiles[0]
		if err := credErrs[p.Name]; err != nil {
			return err
		}
		runner := newWinsrvRunner(flags)
		raw, err := runner.Run(cmd.Context(), p, "service_plan", params)
		if err != nil {
			return scriptExit(err)
		}
		plan, err := decodeRow(raw)
		if err != nil {
			return err
		}
		plan["server"], plan["id"] = p.Name, p.Name+"/"+winsrv.Str(plan["name"])
		if !winsrv.Bool(plan["allowed"]) {
			plan["changed"] = false
			if err := printWinsrv(cmd, flags, plan, "server", "name", "allowed", "refusal"); err != nil {
				return err
			}
			return usageErr(fmt.Errorf("refused: %s %s on %s: %s", action, name, p.Name, winsrv.Str(plan["refusal"])))
		}
		if !(flags.yes || confirm) {
			plan["changed"] = false
			plan["preview"] = true
			if err := printWinsrv(cmd, flags, plan, "server", "name", "allowed", "preview", "state_before"); err != nil {
				return err
			}
			return usageErr(fmt.Errorf("nothing was changed: pass --yes (CLI) or confirm: true (MCP) to %s %s on %s", action, name, p.Name))
		}
		raw, err = runner.Run(winsrv.WithConfirmedMutation(cmd.Context()), p, "service_action", params)
		if err != nil {
			return scriptExit(err)
		}
		result, err := decodeRow(raw)
		if err != nil {
			return err
		}
		result["server"], result["id"] = p.Name, p.Name+"/"+winsrv.Str(result["name"])
		if err := printWinsrv(cmd, flags, result, "server", "name", "changed", "state_before", "state_after"); err != nil {
			return err
		}
		if winsrv.Str(result["state_after"]) != "Running" {
			return apiErr(fmt.Errorf("%s %s on %s: the service is %s after the %s", action, name, p.Name, winsrv.Str(result["state_after"]), action))
		}
		return nil
	}
	cmd.Flags().BoolVar(&confirm, "confirm", false, "Send the change (the MCP form of --yes)")
	return cmd
}

func mustLookup(name string) winsrv.Script {
	s, ok := winsrv.Lookup(name)
	if !ok {
		panic("unregistered script " + name)
	}
	return s
}

func decodeRow(raw []byte) (winsrv.Row, error) {
	var row winsrv.Row
	if err := json.Unmarshal(raw, &row); err != nil || row == nil {
		return nil, errors.New("the server returned an unreadable result")
	}
	return row, nil
}
