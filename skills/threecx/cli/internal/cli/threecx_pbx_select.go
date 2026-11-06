// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-written: --pbx selects one fleet phone system for any command.

package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"threecx-pp-cli/internal/cliutil"
	"threecx-pp-cli/internal/fleet"
)

// pbxSelection is the --pbx flag value for this process; pinnedPBX is the
// registered PBX this process was pinned to (empty when none).
var pbxSelection, pinnedPBX string

// The selector wraps root's PersistentPreRunE from a hook so the generated
// root.go stays untouched. It runs before the generated pre-run applies
// --home, so the per-PBX home and credentials are in place before any
// config, token cache, or local mirror is opened.
func init() {
	registerNovelCommand(func(root *cobra.Command, flags *rootFlags) {
		root.PersistentFlags().StringVar(&pbxSelection, "pbx", "",
			"Run against one registered 3CX phone system (see 'fleet list'; env: THREECX_PBX)")
		next := root.PersistentPreRunE
		root.PersistentPreRunE = func(cmd *cobra.Command, args []string) error {
			if err := holdStdinBody(cmd); err != nil {
				return err
			}
			if err := applyPBXSelection(cmd, flags); err != nil {
				return err
			}
			if next != nil {
				return next(cmd, args)
			}
			return nil
		}
	})
}

// ApplyMCPPBXSelection applies THREECX_PBX to the MCP server, which has no root
// command to run the selector (handfixes.json mcp-honours-pbx-selection): its
// native tools then use the phone system, config and store Declarations showed
// the kit, as its mirrored CLI tools do.
func ApplyMCPPBXSelection() error {
	// The selection's scrub is for what a phone system's credential may meet.
	// These three belong to the server itself, and a fleet child never gets them:
	// the listener's bearer and allowed browser origins, and the companion's path.
	own := map[string]string{}
	for _, name := range []string{"THREECX_MCP_HTTP_TOKEN", "THREECX_MCP_HTTP_ALLOWED_ORIGINS", "THREECX_CLI_PATH"} {
		if v, ok := os.LookupEnv(name); ok {
			own[name] = v
		}
	}
	flags := &rootFlags{}
	if err := applyPBXSelection(&cobra.Command{}, flags); err != nil {
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

// applyPBXSelection points this process at one registered PBX: its FQDN and
// client credential come from the fleet file, its secret from the env var the
// entry names, and its config/token cache/mirror from an isolated home.
func applyPBXSelection(cmd *cobra.Command, flags *rootFlags) error {
	name := strings.TrimSpace(pbxSelection)
	if name == "" {
		name = strings.TrimSpace(os.Getenv("THREECX_PBX")) // fleet.SelectEnv, a literal so the env-schema gate can read it
	}
	if name == "" {
		return nil
	}
	if flags.homePath != "" || flags.configPath != "" {
		return usageErr(fmt.Errorf("--pbx cannot be combined with --home or --config: each PBX already has its own home and config under the fleet directory"))
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
	p, ok := reg.Get(name)
	if !ok && dryRunOK(flags) && requiredInputWaived() {
		// Verify/dogfood harness dry-runs documented --pbx examples on an
		// empty registry; nothing is sent, so leave the selection unset.
		return nil
	}
	if !ok {
		names := reg.Names()
		if len(names) == 0 {
			return usageErr(fmt.Errorf("unknown PBX %q: the fleet is empty; add one with '%s fleet add %s --fqdn <host> --client-id <id>'", name, cmd.Root().Name(), name))
		}
		return usageErr(fmt.Errorf("unknown PBX %q (registered: %s)", name, strings.Join(names, ", ")))
	}
	home := p.Home(base)
	secret := os.Getenv(p.EnvPrefix + "_CLIENT_SECRET")
	// Local-only commands (fleet changes) never reach the PBX, so they do
	// not need its key.
	localOnly := cmd.Annotations["pp:data-source"] == "local"
	if secret == "" && !dryRunOK(flags) && !localOnly {
		return authErr(fmt.Errorf("PBX %q: set %s to its client secret (the API key shown once in Admin Console > Integrations > API)", p.Name, p.SecretVarName()))
	}
	// The registry entry is authoritative for this PBX: keep only what
	// fleet.Scrub allows (the same rule children get), so no other THREECX_*
	// variable or other client's key can redirect or ride along.
	kept := map[string]bool{}
	for _, kv := range fleet.Scrub(os.Environ(), reg, p) {
		k, _, _ := strings.Cut(kv, "=")
		kept[k] = true
	}
	for _, kv := range os.Environ() {
		if k, _, _ := strings.Cut(kv, "="); !kept[k] {
			_ = os.Unsetenv(k)
		}
	}
	if secret != "" {
		_ = os.Setenv("THREECX_CLIENT_SECRET", secret)
	}
	_ = os.Setenv("THREECX_FQDN", p.FQDN)
	// Pin both endpoints from the registry. Env wins over any config file,
	// so nothing on disk can send this PBX's key or token to another host.
	_ = os.Setenv("THREECX_BASE_URL", "https://"+p.FQDN+"/xapi/v1")
	_ = os.Setenv("THREECX_TOKEN_URL", "https://"+p.FQDN+"/connect/token")
	_ = os.Setenv("THREECX_CLIENT_ID", p.ClientID)
	_ = os.Setenv(fleet.SelectEnv, p.Name)
	_ = os.Setenv(fleet.DirEnv, base)
	// An explicit config path inside the PBX home turns off the generated
	// legacy-config fallback, so a global config or token never applies here.
	_ = os.Setenv("THREECX_CONFIG", filepath.Join(home, "config", "config.toml"))
	flags.homePath = home
	pinnedPBX = p.Name
	return nil
}
