// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-written: --instance selects one registered Jamf Pro instance for any command.

package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"jamf-pro-pp-cli/internal/client"
	"jamf-pro-pp-cli/internal/cliutil"
	"jamf-pro-pp-cli/internal/config"
	"jamf-pro-pp-cli/internal/fleet"
	"jamf-pro-pp-cli/internal/jamfauth"
)

// instanceSelection is the --instance flag value for this process;
// pinnedInstance is the registered instance this process acts for (empty
// when none). The identity itself is jamfauth's established value.
var instanceSelection, pinnedInstance string

// The selector wraps root's PersistentPreRunE from a hook so the generated
// root.go stays untouched. It runs before the generated pre-run opens any
// config, token cache or local mirror: first the instance (if any) is
// pinned, then the process identity is established, once.
func init() {
	registerNovelCommand(func(root *cobra.Command, flags *rootFlags) {
		root.PersistentFlags().StringVar(&instanceSelection, "instance", "",
			"Run against one registered Jamf Pro instance (see 'instance list'; env: JAMF_PRO_INSTANCE)")
		next := root.PersistentPreRunE
		root.PersistentPreRunE = func(cmd *cobra.Command, args []string) error {
			// One execution acts as one identity, decided below, once.
			jamfauth.BeginExecution()
			fleet.ReleaseHeld()
			pinnedInstance = ""
			if err := applyInstanceSelection(cmd, flags); err != nil {
				return err
			}
			if err := establishProcessIdentity(cmd, flags); err != nil {
				return err
			}
			if next != nil {
				return next(cmd, args)
			}
			return nil
		}
	})
}

// inheritedIdentity decodes the identity a parent process handed down.
func inheritedIdentity() (jamfauth.Identity, bool, error) {
	raw := strings.TrimSpace(jamfauth.Inherited())
	if raw == "" {
		return jamfauth.Identity{}, false, nil
	}
	id, err := jamfauth.Decode(raw)
	return id, err == nil, err
}

// selectedInstance is the instance this process must act for: the one a
// parent handed down (its identity is used as is, never looked up again),
// else the one named by --instance or JAMF_PRO_INSTANCE.
func selectedInstance() (name string, inherited jamfauth.Identity, ok bool, err error) {
	name = strings.TrimSpace(instanceSelection)
	if name == "" {
		name = strings.TrimSpace(jamfauth.Selected())
	}
	id, has, err := inheritedIdentity()
	if err != nil {
		return "", jamfauth.Identity{}, false, err
	}
	if has && id.Instance() != "" {
		if name != "" && name != id.Instance() {
			return "", jamfauth.Identity{}, false, fmt.Errorf("this process was started for instance %q; instance %q is refused", id.Instance(), name)
		}
		return id.Instance(), id, true, nil
	}
	return name, jamfauth.Identity{}, false, nil
}

// applyInstanceSelection points this process at one registered instance: its
// identity (server, client) is created once from the registry, or taken
// whole from the parent; its secret comes from its JAMF_PRO_FLEET_SECRETS
// entry, and its config/token cache/mirror from an isolated home.
func applyInstanceSelection(cmd *cobra.Command, flags *rootFlags) error {
	name, inherited, hasInherited, err := selectedInstance()
	if err != nil {
		return usageErr(err)
	}
	if name == "" {
		return nil
	}
	// An instance's credentials come from the registry and its env var only.
	// Logging in under --instance would persist a secret into its home.
	if cmd.Parent() != nil && cmd.Parent().Name() == "auth" && (cmd.Name() == "login" || cmd.Name() == "set-token") {
		return usageErr(fmt.Errorf("auth %s is not used with --instance: the instance's client id is in the registry and its secret in %s; use 'instance add' to change them", cmd.Name(), fleet.EntriesEnv))
	}
	if flags.homePath != "" || flags.configPath != "" {
		return usageErr(fmt.Errorf("--instance cannot be combined with --home or --config: each instance already has its own home and config under the fleet directory"))
	}
	id := inherited
	if !hasInherited {
		reg, err := fleet.Load()
		if err != nil {
			return configErr(err)
		}
		p, ok := reg.Get(name)
		if !ok && dryRunOK(flags) && requiredInputWaived() {
			// Verify/dogfood harness dry-runs documented --instance examples on
			// an empty registry; nothing is sent, so leave the selection unset.
			jamfauth.Waive()
			return nil
		}
		if !ok {
			names := reg.Names()
			if len(names) == 0 {
				return usageErr(fmt.Errorf("unknown instance %q: no instances are registered; add one with '%s instance add %s --server <host> --client-id <id>'", name, cmd.Root().Name(), name))
			}
			return usageErr(fmt.Errorf("unknown instance %q (registered: %s)", name, strings.Join(names, ", ")))
		}
		if id, err = jamfauth.ForInstance(p.Name, p.Server, p.ClientID); err != nil {
			return configErr(err)
		}
	}
	secret, err := fleet.SecretFor(id.Instance())
	if err != nil {
		return configErr(err)
	}
	// Local-only commands never reach the server, so they do not need its secret.
	localOnly := cmd.Annotations["pp:data-source"] == "local"
	if secret == "" && !dryRunOK(flags) && !localOnly {
		return authErr(missingSecretErr(id.Instance()))
	}
	home, err := pinInstance(id, secret)
	if err != nil {
		return configErr(err)
	}
	flags.homePath = home
	return nil
}

// establishProcessIdentity establishes, once, the identity a non-instance
// process acts as: the one a parent handed down, else the one its config and
// environment resolve to now. 'auth login' and 'auth set-token' define an
// identity rather than act as one, so they establish none. When nothing
// resolves, none is established and every client is refused by assess.
func establishProcessIdentity(cmd *cobra.Command, flags *rootFlags) error {
	if _, ok := jamfauth.Established(); ok {
		return nil
	}
	if cmd.Parent() != nil && cmd.Parent().Name() == "auth" && (cmd.Name() == "login" || cmd.Name() == "set-token") {
		return nil
	}
	id, has, err := inheritedIdentity()
	if err != nil {
		return configErr(err)
	}
	if has {
		return jamfauth.Establish(id)
	}
	// The generated pre-run applies --home after this; apply it first so the
	// config read here is the one every later client reads.
	if _, err := cliutil.SetHomeOverride(flags.homePath); err != nil {
		return err
	}
	cfg, err := config.Load(flags.configPath)
	if err != nil {
		return nil
	}
	a, err := assess(cfg, cfg.BaseURL)
	if err != nil {
		return nil
	}
	return jamfauth.Establish(a.id)
}

// EstablishMCPIdentity is the MCP server's pre-run (hand-fix in
// cmd/jamf-pro-mcp/main.go): it pins JAMF_PRO_INSTANCE (env scrub,
// instance home) or resolves the configured identity, establishes it once,
// and exports it, so native tools and every mirrored command act as the
// same identity. Any error stops the server: it never runs without one.
func EstablishMCPIdentity() error {
	// The scrub drops every JAMF_PRO_* variable, including the token HTTP
	// callers must present to this MCP server; keep that one.
	callerToken, hasCallerToken := os.LookupEnv(mcpHTTPCallerEnv)
	defer func() {
		if hasCallerToken {
			_ = os.Setenv(mcpHTTPCallerEnv, callerToken)
		}
	}()
	name, inherited, hasInherited, err := selectedInstance()
	if err != nil {
		return err
	}
	if name != "" {
		id := inherited
		if !hasInherited {
			reg, err := fleet.Load()
			if err != nil {
				return err
			}
			p, ok := reg.Get(name)
			if !ok {
				return fmt.Errorf("unknown instance %q in %s (registered: %s)", name, fleet.SelectEnv, strings.Join(reg.Names(), ", "))
			}
			if id, err = jamfauth.ForInstance(p.Name, p.Server, p.ClientID); err != nil {
				return err
			}
		}
		secret, err := fleet.SecretFor(id.Instance())
		if err != nil {
			return err
		}
		if secret == "" {
			return missingSecretErr(id.Instance())
		}
		home, err := pinInstance(id, secret)
		if err != nil {
			return err
		}
		_, err = cliutil.SetHomeOverride(home)
		return err
	}
	if id, has, err := inheritedIdentity(); err != nil {
		return err
	} else if has {
		return jamfauth.Establish(id)
	}
	// Fail closed: the server does not start without an identity.
	cfg, err := config.Load("")
	if err != nil {
		return fmt.Errorf("cannot read the configuration, so no identity can be established: %w", err)
	}
	a, err := assess(cfg, cfg.BaseURL)
	if err != nil {
		// No server configured anywhere (JAMF_PRO_SERVER, a config base URL,
		// an instance): the server starts with no identity, so tools/list
		// works, and every client it builds is refused by the same assess
		// (an unresolved {server} has no origin), so nothing can be sent.
		if strings.Contains(client.ResolvedBaseURL(cfg.BaseURL, cfg.TemplateVars), "{server}") {
			return nil
		}
		return fmt.Errorf("no identity can be established: %w", err)
	}
	if err := jamfauth.Establish(a.id); err != nil {
		return err
	}
	return os.Setenv(jamfauth.IdentityEnv, a.id.Encode())
}

// missingSecretErr is the remedy when JAMF_PRO_FLEET_SECRETS has no entry for name.
func missingSecretErr(name string) error {
	return fmt.Errorf("instance %q: add %s=<client secret of its Jamf Pro API client> to %s (entries separated by commas)", name, name, fleet.EntriesEnv)
}

// mcpHTTPCallerEnv is the MCP server's HTTP caller token (cmd/jamf-pro-mcp).
const mcpHTTPCallerEnv = "JAMF_PRO_MCP_HTTP_TOKEN"

// pinInstance makes this process act as the instance identity id: it holds
// the instance's shared lock (removal and re-registration need it
// exclusively), verifies the registry still holds that exact entry, scrubs
// and pins the environment, establishes the identity and exports it for
// children. It returns the instance's home.
func pinInstance(id jamfauth.Identity, secret string) (string, error) {
	base, err := fleet.BaseDir()
	if err != nil {
		return "", err
	}
	p := fleet.Instance{Name: id.Instance(), Server: id.Host(), ClientID: id.ClientID()}
	if err := fleet.HoldInstance(base, p); err != nil {
		return "", err
	}
	if err := jamfauth.Establish(id); err != nil {
		return "", err
	}
	home := p.Home(base)
	// Keep only what fleet.Scrub allows, so no other JAMF_PRO_* variable or
	// other client's secret can redirect or ride along.
	kept := map[string]bool{}
	for _, kv := range fleet.Scrub(os.Environ()) {
		k, _, _ := strings.Cut(kv, "=")
		kept[k] = true
	}
	for _, kv := range os.Environ() {
		if k, _, _ := strings.Cut(kv, "="); !kept[k] {
			_ = os.Unsetenv(k)
		}
	}
	if secret != "" {
		_ = os.Setenv("JAMF_PRO_CLIENT_SECRET", secret)
		// A child this process starts (an MCP tool's CLI run) inherits the
		// identity and reads its secret from its own entry, and no other.
		_ = os.Setenv(fleet.EntriesEnv, fleet.SecretsFor(p.Name, secret))
	}
	_ = os.Setenv("JAMF_PRO_SERVER", p.Server)
	// Pin both endpoints from the identity: the token endpoint follows the
	// base URL. Env wins over any config file, so nothing on disk can send
	// this instance's secret or token elsewhere.
	_ = os.Setenv("JAMF_PRO_BASE_URL", id.Server())
	_ = os.Setenv("JAMF_PRO_CLIENT_ID", p.ClientID)
	_ = os.Setenv(fleet.SelectEnv, p.Name)
	_ = os.Setenv(fleet.DirEnv, base)
	_ = os.Setenv(jamfauth.IdentityEnv, id.Encode())
	// An explicit config path inside the instance home turns off the generated
	// legacy-config fallback, so a global config or token never applies here.
	_ = os.Setenv("JAMF_PRO_CONFIG", filepath.Join(home, "config", "config.toml"))
	pinnedInstance = p.Name
	return home, nil
}
