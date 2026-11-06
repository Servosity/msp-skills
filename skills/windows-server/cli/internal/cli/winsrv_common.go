// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
//
// Hand-written (NOT generated): the Windows Server wiring. Recorded in
// handfixes.json (server-profiles, credkit-wiring).
//
//   - the root --server flag picks a configured server by name; it can never
//     carry a host. --allow-http is CLI-only and refused under MCP.
//   - the `auth set-credentials` password argument is held the moment Cobra
//     hands it over (credkit.Start holds every other credential source).

package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/spf13/cobra"

	"windows-server-pp-cli/internal/cliutil"
	"windows-server-pp-cli/internal/config"
	"windows-server-pp-cli/internal/credkit"
	"windows-server-pp-cli/internal/store"
	"windows-server-pp-cli/internal/winsrv"
)

var (
	winsrvServer    string
	winsrvAllowHTTP bool
	// winsrvFlags are this invocation's root flags, for the virtual transport.
	winsrvFlags *rootFlags

	// newWinsrvRunner builds the runner for one invocation. Tests replace it
	// with a winsrv.FixtureRunner.
	newWinsrvRunner = func(flags *rootFlags) winsrv.Runner {
		r := winsrv.WinRMRunner{AllowHTTP: winsrvAllowHTTP}
		if flags != nil && flags.timeoutExplicit {
			r.Timeout = flags.timeout
		}
		return r
	}
	winsrvNow = time.Now
)

func init() {
	// The local store follows the config file, not the shared password: a
	// rotated password keeps the health, events and inventory history, and
	// two configs (two clients) never share one.
	// Without a config file there are no servers, and the generated default applies.
	config.StoreScopeIdentity = func(c *config.Config) string {
		if c.Path == "" {
			return ""
		}
		if _, err := os.Stat(c.Path); err != nil {
			return ""
		}
		return "windows-server:config:" + winsrv.CanonicalPath(c.Path)
	}
	registerClientHook(installVirtualTransport)
	registerNovelCommand(attachWinsrv)
	doctorReachabilityHook = winsrvDoctorReachability
	doctorAuthConfiguredHook = winsrvDoctorAuthConfigured
}

// attachWinsrv adds the root flags and the hand-written commands.
func attachWinsrv(root *cobra.Command, flags *rootFlags) {
	winsrvFlags = flags
	pf := root.PersistentFlags()
	pf.StringVar(&winsrvServer, "server", "", "Server name from the config file's [[servers]] list (default: the only server, or the one marked default = true)")
	pf.BoolVar(&winsrvAllowHTTP, "allow-http", false, "Allow a server configured with transport = \"http\" (WinRM over plain HTTP, port 5985). CLI only; refused under MCP")
	_ = pf.MarkHidden("allow-http") // hidden flags are never offered as MCP tool arguments
	// A server without the optional role answers not found (exit 3): an
	// expected answer for these reads, not a failure.
	for _, name := range []string{"hyperv-vms", "defender"} {
		if c, _, err := root.Find([]string{name}); err == nil && c != root {
			if c.Annotations == nil {
				c.Annotations = map[string]string{}
			}
			c.Annotations["pp:typed-exit-codes"] = "0,3"
		}
	}

	orig := root.PersistentPreRunE
	root.PersistentPreRunE = func(cmd *cobra.Command, args []string) error {
		if winsrvAllowHTTP && cliutil.IsMCPChildEnv() {
			return usageErr(errors.New("--allow-http is refused under MCP: plain-HTTP WinRM can only be enabled by an operator on the command line"))
		}
		if winsrvServer != "" && !winsrv.ProfileNameRE.MatchString(winsrvServer) {
			return usageErr(fmt.Errorf("--server must be a configured server name (lowercase letters, digits, dashes), not a host"))
		}
		if orig != nil {
			if err := orig(cmd, args); err != nil {
				return err
			}
		}
		return nil
	}
	if c, _, err := root.Find([]string{"auth", "set-credentials"}); err == nil && c.Name() == "set-credentials" {
		holdPasswordArg(c)
	}

	for _, c := range root.Commands() {
		switch c.Name() {
		case "services":
			addNovelCommandIfAbsent(c, newServicesStartCmd(flags))
		case "inventory":
			addNovelCommandIfAbsent(c, newInventorySnapshotCmd(flags))
		}
	}
	addNovelCommandIfAbsent(root, newServersCmd(flags))
}

// winsrvServers loads the configured servers, picks the ones this command
// acts on, and resolves each one's credential. A server whose credential is
// missing is returned with its error in credErrs, so one bad entry does not
// stop a run across every server.
func winsrvServers(flags *rootFlags, all bool) (profiles []winsrv.Profile, credErrs map[string]error, err error) {
	cfg, err := config.Load(flags.configPath)
	if err != nil {
		return nil, nil, configErr(err)
	}
	return serversFromConfig(cfg, winsrvServer, all)
}

func serversFromConfig(cfg *config.Config, name string, all bool) ([]winsrv.Profile, map[string]error, error) {
	all = all && name == ""
	profiles, err := winsrv.LoadProfiles(cfg.Path)
	if err != nil {
		return nil, nil, configErr(err)
	}
	sel, err := winsrv.Select(profiles, name, all)
	if err != nil {
		return nil, nil, usageErr(err)
	}
	credPath, err := cliutil.CredentialsFilePath()
	if err != nil {
		return nil, nil, configErr(err)
	}
	stored, err := winsrv.LoadServerCredentials(winsrv.ServerCredentialsPath(credPath))
	if err != nil {
		return nil, nil, configErr(err)
	}
	shared := winsrv.Shared{Username: cfg.WindowsServerUsername, Password: cfg.WindowsServerPassword, Source: cfg.CredentialSource}
	credErrs := map[string]error{}
	for i := range sel {
		if err := winsrv.ResolveCredentials(&sel[i], stored, shared); err != nil {
			credErrs[sel[i].Name] = configErr(err)
		}
	}
	return sel, credErrs, nil
}

// winsrvDoctorReachability fills doctor's api and credentials rows. The base
// URL is the virtual host this process answers itself, so doctor's generic
// GET / would read an in-process reply as "reachable" without contacting any
// server. It reports what is configured instead and points at health, which
// reaches every server over WinRM. Any other base URL (the verifier's
// loopback mock) keeps the generic probe.
func winsrvDoctorReachability(cfg *config.Config) (api, credentials string, ok bool) {
	if u, err := url.Parse(strings.TrimSpace(cfg.BaseURL)); err != nil || u.Host != VirtualHost {
		if cliutil.IsVerifyEnv() && loopbackBase(cfg.BaseURL) {
			return "", "", false // the verifier's loopback mock keeps the generic probe
		}
		// The transport refuses every host but the virtual one, so a generic
		// GET here can only fail; say why instead of "API unreachable".
		return "refused: base_url must stay " + "https://" + VirtualHost + "; this CLI talks only to the servers named in its config file",
			"WARN not verified: no argument-free GET endpoint over HTTP; run 'windows-server-cli health' to check each server's credential over WinRM", true
	}
	profiles, credErrs, err := serversFromConfig(cfg, winsrvServer, true)
	switch {
	case errors.Is(err, winsrv.ErrNoServers):
		return "not configured (add a [[servers]] entry (name, host) to the config file)", "", true
	case err != nil:
		return "refused: " + err.Error(), "", true
	case len(profiles) == 0:
		return "not configured (add a [[servers]] entry (name, host) to the config file)", "", true
	}
	what, them := fmt.Sprintf("%d servers configured", len(profiles)), "each one"
	if len(profiles) == 1 {
		what, them = "server "+profiles[0].Name+" configured", "it"
	}
	api = what + ", not verified here: run 'windows-server-cli health' to reach " + them + " over WinRM"
	var missing []string
	for _, p := range profiles {
		if credErrs[p.Name] != nil {
			missing = append(missing, p.Name)
		}
	}
	if len(missing) > 0 {
		return api, "missing for " + strings.Join(missing, ", ") + ": run 'windows-server-cli servers list' to see where each server's credential comes from", true
	}
	return api, "present, not verified. Run `windows-server-cli health` to confirm each server's credential end-to-end.", true
}

// winsrvDoctorAuthConfigured tells doctor that auth is configured when every
// selected server has its own stored credential (servers login) and the
// shared WINDOWS_SERVER_USERNAME/PASSWORD pair is not set. doctor calls it only
// when the shared pair is absent.
func winsrvDoctorAuthConfigured() (bool, string) {
	if winsrvFlags == nil {
		return false, ""
	}
	cfg, err := config.Load(winsrvFlags.configPath)
	if err != nil {
		return false, ""
	}
	profiles, credErrs, err := serversFromConfig(cfg, winsrvServer, true)
	if err != nil || len(profiles) == 0 {
		return false, ""
	}
	for _, p := range profiles {
		if credErrs[p.Name] != nil {
			return false, ""
		}
	}
	return true, "per-server credential store"
}

// serverResult is one server's outcome in a multi-server run.
type serverResult struct {
	profile winsrv.Profile
	value   any
	err     error
}

// eachServer runs fn on every profile, four at a time, and returns the
// results in profile order. A profile with a credential error is not run.
func eachServer(ctx context.Context, profiles []winsrv.Profile, credErrs map[string]error, fn func(context.Context, winsrv.Profile) (any, error)) []serverResult {
	out := make([]serverResult, len(profiles))
	sem := make(chan struct{}, 4)
	var wg sync.WaitGroup
	for i, p := range profiles {
		out[i].profile = p
		if err := credErrs[p.Name]; err != nil {
			out[i].err = err
			continue
		}
		wg.Add(1)
		go func(i int, p winsrv.Profile) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			out[i].value, out[i].err = fn(ctx, p)
		}(i, p)
	}
	wg.Wait()
	return out
}

// errorRow is the row a multi-server list carries for a server it could not read.
func errorRow(server string, err error) winsrv.Row {
	return winsrv.Row{"server": server, "id": server + "/error", "error": err.Error()}
}

// allFailed returns the first error when every server failed, so a run that
// read nothing exits non-zero.
func allFailed(results []serverResult) error {
	for _, r := range results {
		if r.err == nil {
			return nil
		}
	}
	if len(results) > 0 {
		return results[0].err
	}
	return nil
}

// printWinsrv prints v through the standard output flags, keeping keep
// under --compact/--agent.
func printWinsrv(cmd *cobra.Command, flags *rootFlags, v any, keep ...string) error {
	raw, err := json.Marshal(v)
	if err != nil {
		return err
	}
	var decoded any
	if err := json.Unmarshal(raw, &decoded); err != nil {
		return err
	}
	return printJSONFilteredKeep(cmd.OutOrStdout(), decoded, flags, keep...)
}

// openWinsrvStore opens the local store for writing.
func openWinsrvStore(ctx context.Context) (*store.Store, error) {
	return store.OpenWithContext(ctx, defaultDBPath("windows-server-cli"))
}

// scriptExit maps a winsrv error to the CLI's exit-code classes.
func scriptExit(err error) error {
	var se *winsrv.ScriptError
	var ce *cliError
	switch {
	case err == nil:
		return nil
	case errors.As(err, &ce):
		return err
	case errors.Is(err, winsrv.ErrNotConfirmed):
		return usageErr(err)
	case errors.As(err, &se) && se.Kind == winsrv.KindNotFound:
		return notFoundErr(err)
	case errors.As(err, &se) && se.Kind == winsrv.KindAccessDenied:
		return authErr(err)
	case errors.As(err, &se) && se.Kind == winsrv.KindInvalidArg:
		return usageErr(err)
	case isWinRMAuth(err):
		return authErr(err)
	}
	return apiErr(err)
}

// winsrvTarget names the servers a command would contact, for --dry-run,
// without reading the config.
func winsrvTarget() string {
	if winsrvServer != "" {
		return "server " + winsrvServer
	}
	return "every configured server"
}

// holdPasswordArg holds the password `auth set-credentials` takes as its
// second argument the moment Cobra hands it over, before the command runs or
// prints. A positional argument is no source the kit can read at startup.
func holdPasswordArg(c *cobra.Command) {
	validate := c.Args
	c.Args = func(cmd *cobra.Command, args []string) error {
		if len(args) > 1 {
			credkit.Hold("minted:set-credentials", args[1:]...) // Hand-fix (handfixes.json credkit-wiring)
		}
		if validate == nil {
			return nil
		}
		return validate(cmd, args)
	}
}
