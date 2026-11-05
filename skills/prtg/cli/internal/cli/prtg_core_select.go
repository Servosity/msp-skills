// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-written: --core selects one registered PRTG core for any command.

package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"prtg-pp-cli/internal/cliutil"
	"prtg-pp-cli/internal/fleet"
)

// coreSelection is the --core flag value for this process; pinnedCore is the
// registered core this process was pinned to (empty when none).
var coreSelection, pinnedCore string

// The variables package fleet names, spelled as literals here so every read
// in this package names its variable (check_env_schema.py resolves constants
// within one package only). TestFleetEnvNamesMatchPackageFleet keeps them
// equal to fleet.SelectEnv and fleet.ChildEnv.
const (
	coreSelectEnv = "PRTG_CORE"
	fleetChildEnv = "PP_PRTG_FLEET_CHILD"
)

// The selector wraps root's PersistentPreRunE from a hook so the generated
// root.go stays untouched. It runs before the generated pre-run applies
// --home, so the per-core home and key are in place before any config,
// cache, or local mirror is opened.
func init() {
	registerNovelCommand(func(root *cobra.Command, flags *rootFlags) {
		root.PersistentFlags().StringVar(&coreSelection, "core", "",
			"Run against one registered PRTG core (see 'core list'; env: PRTG_CORE)")
		next := root.PersistentPreRunE
		root.PersistentPreRunE = func(cmd *cobra.Command, args []string) error {
			if err := holdStdinBody(cmd); err != nil {
				return err
			}
			if err := applyCoreSelection(cmd, flags); err != nil {
				return err
			}
			if next != nil {
				return next(cmd, args)
			}
			return nil
		}
	})
}

// ApplyMCPCoreSelection applies PRTG_CORE to the MCP server, which has no root
// command to run the selector (handfixes.json mcp-honours-core-selection): its
// native tools then use the core, config and store Declarations showed the kit,
// as its mirrored CLI tools do.
func ApplyMCPCoreSelection() error {
	// The selection's scrub is for what a core's key may meet. These three belong
	// to the server itself, and a fleet child never gets them: the listener's
	// bearer and allowed browser origins, and the companion's path.
	own := map[string]string{}
	for _, name := range []string{"PRTG_MCP_HTTP_TOKEN", "PRTG_MCP_HTTP_ALLOWED_ORIGINS", "PRTG_CLI_PATH"} {
		if v, ok := os.LookupEnv(name); ok {
			own[name] = v
		}
	}
	flags := &rootFlags{}
	if err := applyCoreSelection(&cobra.Command{}, flags); err != nil {
		return err
	}
	for name, v := range own {
		_ = os.Setenv(name, v)
	}
	if flags.homePath == "" {
		return nil
	}
	_, err := cliutil.SetHomeOverride(flags.homePath)
	return err
}

// applyCoreSelection points this process at one registered core: its server
// comes from the fleet file, its API key from its PRTG_FLEET_KEYS entry, and
// its config, cache and mirror from an isolated home.
func applyCoreSelection(cmd *cobra.Command, flags *rootFlags) error {
	name := strings.TrimSpace(coreSelection)
	if name == "" {
		name = strings.TrimSpace(os.Getenv(coreSelectEnv))
	}
	if name == "" {
		return nil
	}
	// Registry management never runs pinned: an exported PRTG_CORE naming a
	// core that is not registered yet must not block 'core add'.
	for c := cmd; c != nil; c = c.Parent() {
		if c.Name() == "core" && c.Parent() != nil && c.Parent().Parent() == nil {
			return nil
		}
	}
	if flags.homePath != "" || flags.configPath != "" {
		return usageErr(fmt.Errorf("--core cannot be combined with --home or --config: each core already has its own home and config under the fleet directory"))
	}
	// Resolve the fleet directory before anything below can move it.
	base, err := fleet.BaseDir()
	if err != nil {
		return configErr(err)
	}
	reg, err := fleet.Load()
	if err != nil {
		return configErr(err)
	}
	c, ok := reg.Get(name)
	if !ok && dryRunOK(flags) && requiredInputWaived() {
		// Verify/dogfood harness dry-runs documented --core examples on an
		// empty registry; nothing is sent, so leave the selection unset.
		return nil
	}
	if !ok {
		names := reg.Names()
		if len(names) == 0 {
			return usageErr(fmt.Errorf("unknown core %q: the fleet is empty; add one with '%s core add %s --server <host[:port]>'", name, cmd.Root().Name(), name))
		}
		return usageErr(fmt.Errorf("unknown core %q (registered: %s)", name, strings.Join(names, ", ")))
	}
	home := c.Home(base)
	// A value that does not parse yields no key at all, never a partial set.
	keys, keysErr := fleet.Keys()
	key := keys[c.Name]
	// Local-only commands never reach the core, so they do not need its key.
	localOnly := cmd.Annotations["pp:data-source"] == "local"
	if key == "" && !dryRunOK(flags) && !localOnly {
		if keysErr != nil {
			return authErr(fmt.Errorf("core %q: %w", c.Name, keysErr))
		}
		return authErr(fmt.Errorf("core %q: add %s=<API key> to PRTG_FLEET_KEYS (PRTG: Account Settings > My Account > API Keys)", c.Name, c.Name))
	}
	// The registry entry is authoritative for this core: keep only what
	// fleet.Scrub allows (the same rule children get), so no other PRTG_*
	// variable can redirect, and PRTG_FLEET_KEYS shrinks to this core's own
	// entry, so no other client's key rides along.
	kept := map[string]string{}
	for _, kv := range fleet.Scrub(os.Environ(), c, key) {
		k, v, _ := strings.Cut(kv, "=")
		kept[k] = v
	}
	for _, kv := range os.Environ() {
		if k, _, _ := strings.Cut(kv, "="); !hasKey(kept, k) {
			_ = os.Unsetenv(k)
		}
	}
	if own, ok := kept[fleet.KeysEnv]; ok {
		_ = os.Setenv(fleet.KeysEnv, own)
	}
	if key != "" {
		_ = os.Setenv("PRTG_API_KEY", key)
	}
	// Pin the endpoint from the registry. Env wins over any config file, so
	// nothing on disk can send this core's key to another host.
	if c.InsecureTLS {
		// Per-core opt-in recorded by 'core add --insecure-tls'.
		_ = os.Setenv("PRTG_SKIP_TLS_VERIFY", "true")
	}
	_ = os.Setenv("PRTG_SERVER", c.Server)
	_ = os.Setenv("PRTG_BASE_URL", c.BaseURL())
	_ = os.Setenv(fleet.SelectEnv, c.Name)
	_ = os.Setenv(fleet.DirEnv, base)
	// An explicit config path inside the core home turns off the generated
	// legacy-config fallback, so a global config or token never applies here.
	_ = os.Setenv("PRTG_CONFIG", filepath.Join(home, "config", "config.toml"))
	flags.homePath = home
	pinnedCore = c.Name
	return nil
}

// hasKey reports whether m has an entry for k, even an empty one.
func hasKey(m map[string]string, k string) bool {
	_, ok := m[k]
	return ok
}
