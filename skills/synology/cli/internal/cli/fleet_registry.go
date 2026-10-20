// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// HAND-AUTHORED. fleet add / list / remove: the per-NAS registry.
// pp:data-source local

package cli

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"synology-pp-cli/internal/dsm"
	"synology-pp-cli/internal/fleet"
)

func init() {
	registerNovelCommand(func(root *cobra.Command, flags *rootFlags) {
		fleetCmd, _, err := root.Find([]string{"fleet"})
		if err != nil || fleetCmd == nil || fleetCmd.Name() != "fleet" {
			return
		}
		addNovelCommandIfAbsent(fleetCmd, newFleetAddCmd(flags))
		addNovelCommandIfAbsent(fleetCmd, newFleetListCmd(flags))
		addNovelCommandIfAbsent(fleetCmd, newFleetRemoveCmd(flags))
	})
}

type fleetAddResult struct {
	NAS            fleet.NAS       `json:"nas"`
	Repinned       bool            `json:"repinned,omitempty"`
	PreviousPin    string          `json:"previous_tls_fingerprint,omitempty"`
	RegistryPath   string          `json:"registry_path"`
	EnvVars        map[string]bool `json:"env_vars_set"`
	CheckedLogin   bool            `json:"checked_login,omitempty"`
	ABBAvailable   *bool           `json:"abb_available,omitempty"`
	HyperBackupAPI *bool           `json:"hyper_backup_available,omitempty"`
	Warning        string          `json:"warning,omitempty"`
}

func newFleetAddCmd(flags *rootFlags) *cobra.Command {
	var nasURL, client, envPrefix, fingerprint string
	var pinCert, insecure, check bool
	cmd := &cobra.Command{
		Use:   "add [name]",
		Short: "Register a client NAS: its URL, client name and the env-var prefix holding its login",
		Long: strings.Trim(`
Register a client NAS for the fleet commands. The login is never stored: the
NAS reads <prefix>_ACCOUNT, <prefix>_PASSWORD and, for two-factor accounts,
<prefix>_DEVICE_ID (or a one-time <prefix>_OTP_CODE) from the environment.
The prefix defaults to SYNOLOGY_<NAME>.

Most NAS units serve DSM's self-signed certificate: --pin-cert records its
SHA-256 fingerprint now and verifies exactly that certificate afterwards.
--check performs a test login and reports whether Active Backup for Business
is installed.

Re-pin a registered NAS after it renewed its certificate (DSM's self-signed
one, or Let's Encrypt every ~90 days) with 'fleet add <name> --pin-cert' (or
--tls-fingerprint): the NAS keeps its address, client, env-var prefix and
mirrored history; only the pin changes. A different --url is refused there:
a new address is a new NAS (fleet remove, then fleet add).

Registry administration is operator-only: fleet add and fleet remove are not
MCP tools.`, "\n"),
		Example: strings.Trim(`
  synology-cli fleet add acme --url https://nas.acme.example:5001 --client "Acme Corp"
  synology-cli fleet add acme --url https://192.168.10.5:5001 --pin-cert --check
  synology-cli fleet add lab --url http://10.0.0.20:5000 --env-prefix LAB_NAS
  synology-cli fleet add acme --pin-cert`, "\n"),
		// mcp:hidden: an MCP caller must never choose the address (and the
		// env-var prefix, so which password) a login is sent to.
		Annotations: map[string]string{"mcp:read-only": "false", "mcp:hidden": "true", "pp:happy-args": "name=acme;--url=https://nas.acme.example:5001;--dry-run"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 && cmd.Flags().NFlag() == 0 {
				return cmd.Help()
			}
			if dryRunOK(flags) {
				if nasURL != "" {
					if _, err := fleet.OriginURL(nasURL); err != nil {
						return usageErr(fmt.Errorf("--url: %w", err)) // never preview an address carrying a password
					}
				}
				return writeDryRun(cmd.OutOrStdout(), flags, "fleet add")
			}
			if len(args) != 1 {
				_ = cmd.Usage()
				return usageErr(fmt.Errorf("a NAS name is required, for example: fleet add acme --url https://nas.acme.example:5001"))
			}
			name := strings.ToLower(args[0])
			if !fleet.ValidName(name) {
				return usageErr(fmt.Errorf("NAS name %q must be lowercase letters, digits, '.', '_' or '-'", args[0]))
			}
			// A pin flag without --url re-pins a registered NAS (checked under
			// the registry lock below); every other add needs --url.
			repin := pinCert || fingerprint != ""
			origin := ""
			if nasURL != "" || !repin {
				var err error
				if origin, err = fleet.OriginURL(nasURL); err != nil {
					return usageErr(fmt.Errorf("--url: %w", err))
				}
			}
			explicitPrefix := cmd.Flags().Changed("env-prefix")
			if envPrefix == "" {
				envPrefix = fleet.DefaultEnvPrefix(name)
			}
			if pinCert && fingerprint != "" {
				return usageErr(fmt.Errorf("--pin-cert and --tls-fingerprint both set; use one"))
			}
			if strings.HasPrefix(origin, "http://") {
				if pinCert || fingerprint != "" {
					return usageErr(errors.New("a certificate pin needs an https:// --url; plain HTTP has no certificate"))
				}
				fmt.Fprintln(cmd.ErrOrStderr(), "warning: http:// sends the DSM password and session in clear text; prefer https://<nas>:5001 with --pin-cert")
			}
			// Validate the pin as typed: "sha256/" or ":" normalizes to nothing
			// and must not silently register an unpinned NAS.
			if fingerprint != "" {
				if err := dsm.ApplyTLS(&tls.Config{}, fingerprint, false); err != nil {
					return usageErr(err)
				}
			}
			n := fleet.NAS{Name: name, URL: origin, Client: client, EnvPrefix: envPrefix,
				TLSFingerprint: dsm.NormalizeFingerprint(fingerprint), Insecure: insecure}
			// The registry lock is held for the whole add, --check login included:
			// a concurrent add waits, so names and credential prefixes are
			// validated against the registry as it will be saved, and no
			// password is sent for a registration that would then be refused.
			_, _, path, err := fleetNAS(flags)
			if err != nil {
				return err
			}
			ctx, cancel := boundCtx(cmd.Context(), flags)
			defer cancel()
			var prev fleet.NAS
			var replacing bool
			res := fleetAddResult{}
			err = fleet.UpdateRegistry(path, func(r *fleet.Registry) error {
				registered, exists := r.Get(n.Name)
				switch {
				case repin && exists:
					// Re-pin: same NAS, new certificate. Keep everything the
					// operator registered (address, client, prefix) unless a
					// flag says otherwise; the mirror rows stay, because they
					// are tied to the unchanged address.
					if origin != "" && origin != registered.URL {
						return usageErr(fmt.Errorf("NAS %q is registered at %s; re-pinning keeps that address and refuses --url %s (a different address is a different NAS: run 'synology-cli fleet remove %s', then 'synology-cli fleet add %s --url %s --pin-cert')", n.Name, registered.URL, origin, n.Name, n.Name, origin))
					}
					if !strings.HasPrefix(registered.URL, "https://") {
						return usageErr(errors.New("a certificate pin needs an https:// --url; plain HTTP has no certificate"))
					}
					n.URL = registered.URL
					if !cmd.Flags().Changed("client") {
						n.Client = registered.Client
					}
					if !explicitPrefix {
						n.EnvPrefix = registered.EnvPrefix
					}
					res.Repinned, res.PreviousPin = true, registered.TLSFingerprint
				case n.URL == "":
					return usageErr(fmt.Errorf("--url: %s is not registered; a new NAS needs its address, for example: fleet add %s --url https://<nas>:5001 --pin-cert", n.Name, n.Name))
				}
				if err := prefixClash(r, n, explicitPrefix); err != nil {
					return err
				}
				if pinCert {
					fp, err := dsm.FetchFingerprint(ctx, n.URL)
					if err != nil {
						return apiErr(fmt.Errorf("reading the NAS certificate: %w", err))
					}
					n.TLSFingerprint = fp
				}
				if n.TLSFingerprint != "" {
					if err := dsm.ApplyTLS(&tls.Config{}, n.TLSFingerprint, false); err != nil {
						return usageErr(err)
					}
				}
				res.NAS, res.EnvVars = n, n.EnvStatus()
				if check {
					info, warn, err := checkNAS(ctx, flags, n)
					if err != nil {
						return err
					}
					res.CheckedLogin, res.Warning = true, warn
					if warn != "" {
						fmt.Fprintln(cmd.ErrOrStderr(), "warning:", warn)
					}
					abb, hb := info[fleet.PathABBTasks], info[fleet.PathHyperBackupTasks]
					res.ABBAvailable, res.HyperBackupAPI = &abb, &hb
				}
				prev, replacing = r.Get(n.Name)
				r.Put(n)
				return nil
			})
			if err != nil {
				var ce *cliError
				if As(err, &ce) {
					return err
				}
				return configErr(err)
			}
			if replacing && prev.URL != n.URL {
				// The mirror still holds the old NAS's rows; reports read them as
				// not synced (URL mismatch) and the next sync replaces them in the
				// same transaction that commits the new data.
				fmt.Fprintf(cmd.ErrOrStderr(), "note: %s now points at %s; its mirrored history from %s is replaced on the next sync\n", n.Name, n.URL, prev.URL)
			}
			res.RegistryPath = path
			if !wantsHumanTable(cmd.OutOrStdout(), flags) {
				return printJSONFiltered(cmd.OutOrStdout(), res, flags)
			}
			if res.Repinned {
				old := res.PreviousPin
				if old == "" {
					old = "(none)"
				}
				fmt.Fprintf(cmd.OutOrStdout(), "re-pinned %s (%s): %s -> %s\n", n.Name, n.URL, old, n.TLSFingerprint)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "registered %s (%s) in %s\n", n.Name, n.URL, path)
			for k, set := range res.EnvVars {
				state := "set"
				if !set {
					state = "NOT SET"
				}
				fmt.Fprintf(cmd.OutOrStdout(), "  %s: %s\n", k, state)
			}
			if res.ABBAvailable != nil {
				fmt.Fprintf(cmd.OutOrStdout(), "  login ok; Active Backup for Business API: %v\n", *res.ABBAvailable)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&nasURL, "url", "", "NAS address with scheme and port, for example https://192.168.1.50:5001")
	cmd.Flags().StringVar(&client, "client", "", "MSP client name shown in fleet reports")
	cmd.Flags().StringVar(&envPrefix, "env-prefix", "", "Env-var prefix for this NAS's login (<prefix>_ACCOUNT, <prefix>_PASSWORD, <prefix>_DEVICE_ID); default SYNOLOGY_<NAME>")
	cmd.Flags().BoolVar(&pinCert, "pin-cert", false, "Fetch the NAS certificate now and trust only that certificate from then on (for self-signed DSM certificates)")
	cmd.Flags().StringVar(&fingerprint, "tls-fingerprint", "", "SHA-256 fingerprint of the NAS certificate to pin (hex, colons allowed)")
	cmd.Flags().BoolVar(&insecure, "insecure", false, "Skip TLS verification for this NAS (prefer --pin-cert)")
	cmd.Flags().BoolVar(&check, "check", false, "Log in now and report whether Active Backup for Business and Hyper Backup APIs are available")
	return cmd
}

// prefixClash refuses a NAS whose credential prefix another NAS already uses,
// unless --env-prefix chose it deliberately.
func prefixClash(r *fleet.Registry, n fleet.NAS, explicit bool) error {
	for _, other := range r.NAS {
		if other.Name != n.Name && other.EnvPrefix == n.EnvPrefix && !explicit {
			return usageErr(fmt.Errorf("NAS %q already uses credential prefix %s; pass --env-prefix to choose another (or the same one deliberately)", other.Name, n.EnvPrefix))
		}
	}
	return nil
}

// checkNAS logs in and asks SYNO.API.Info for the backup APIs. warn reports a
// trusted-device token that could not be cached (the next login needs a code).
func checkNAS(ctx context.Context, flags *rootFlags, n fleet.NAS) (info map[string]bool, warn string, err error) {
	if !n.Credentials().Complete() {
		return nil, "", authErr(fmt.Errorf("set %s_ACCOUNT and %s_PASSWORD before --check", n.EnvPrefix, n.EnvPrefix))
	}
	c, err := dsmClient(flags, n)
	if err != nil {
		return nil, "", err
	}
	defer func() { _ = c.Logout(context.Background()) }()
	info, err = apiInfo(ctx, c, fleet.PathABBTasks, fleet.PathHyperBackupTasks)
	if err != nil {
		return nil, "", redactErr(c, authErr(fmt.Errorf("login check against %s failed: %w", n.URL, err)))
	}
	if c.DeviceIDErr != nil {
		warn = "could not cache the trusted-device token; the next login will need a one-time code again: " + c.DeviceIDErr.Error()
	}
	return info, warn, nil
}

type fleetListRow struct {
	Name           string          `json:"name"`
	Client         string          `json:"client,omitempty"`
	URL            string          `json:"url"`
	EnvPrefix      string          `json:"env_prefix"`
	EnvVars        map[string]bool `json:"env_vars_set"`
	TLS            string          `json:"tls"`
	LastSyncAt     int64           `json:"last_sync_at,omitempty"`
	LastSyncError  string          `json:"last_sync_error,omitempty"`
	ImplicitConfig bool            `json:"implicit_config,omitempty"`
}

// ReportIdentity is what --quiet prints for the row.
func (r fleetListRow) ReportIdentity() string { return r.Name }

func newFleetListCmd(flags *rootFlags) *cobra.Command {
	var dbPath string
	cmd := &cobra.Command{
		Use:         "list",
		Short:       "List registered NAS units, which credential env vars are set, TLS mode and last sync",
		Example:     "  synology-cli fleet list --agent",
		Annotations: map[string]string{"mcp:read-only": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return writeDryRun(cmd.OutOrStdout(), flags, "fleet list")
			}
			list, reg, _, err := fleetNAS(flags)
			if err != nil {
				return err
			}
			rows := make([]fleetListRow, 0, len(list))
			path, err := fleetDBPath(dbPath)
			if err != nil {
				return err
			}
			db, closeDB, err := openFleetRead(cmd.Context(), path, false)
			if err != nil {
				return err
			}
			defer closeDB()
			states := map[string]fleet.SyncState{}
			if db != nil {
				if states, err = fleet.LoadSyncStates(cmd.Context(), db); err != nil && !strings.Contains(err.Error(), "no such table") {
					return err
				}
			}
			for _, n := range list {
				st := states[n.Name]
				rows = append(rows, fleetListRow{Name: n.Name, Client: n.Client, URL: n.URL, EnvPrefix: n.EnvPrefix,
					EnvVars: n.EnvStatus(), TLS: fleetTLSMode(n), LastSyncAt: st.SucceededAt, LastSyncError: st.Error,
					ImplicitConfig: len(reg.NAS) == 0})
			}
			if len(rows) == 0 && wantsHumanTable(cmd.OutOrStdout(), flags) {
				noFleetHint(cmd.ErrOrStderr())
			}
			return printFleetRows(cmd, flags, rows, "no NAS registered")
		},
	}
	cmd.Flags().StringVar(&dbPath, "db", "", "Fleet mirror path (default <data dir>/fleet.db)")
	return cmd
}

// fleetTLSMode names how a NAS's connection is protected. Plain http:// has
// no TLS at all, so it never reads as "verified" (or pinned/insecure, which
// are https:// modes).
func fleetTLSMode(n fleet.NAS) string {
	switch {
	case !strings.HasPrefix(strings.ToLower(n.URL), "https://"):
		return "none (plain http)"
	case n.TLSFingerprint != "":
		return "pinned"
	case n.Insecure:
		return "insecure"
	}
	return "verified"
}

func newFleetRemoveCmd(flags *rootFlags) *cobra.Command {
	var keepData bool
	var dbPath string
	// mcp:hidden: registry administration is operator-only (see fleet add).
	cmd := &cobra.Command{
		Use:         "remove [name]",
		Short:       "Unregister a NAS and delete its rows from the local mirror",
		Example:     "  synology-cli fleet remove acme",
		Annotations: map[string]string{"mcp:read-only": "false", "mcp:hidden": "true", "pp:happy-args": "name=acme;--dry-run"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 && cmd.Flags().NFlag() == 0 {
				return cmd.Help()
			}
			if dryRunOK(flags) {
				return writeDryRun(cmd.OutOrStdout(), flags, "fleet remove")
			}
			if len(args) != 1 {
				_ = cmd.Usage()
				return usageErr(fmt.Errorf("a NAS name is required"))
			}
			_, _, path, err := fleetNAS(flags)
			if err != nil {
				return err
			}
			name := strings.ToLower(args[0])
			purged := false
			// Unregister and purge under the registry lock, so a concurrent
			// re-add of the same name cannot lose the rows its sync writes.
			if err := fleet.UpdateRegistry(path, func(r *fleet.Registry) error {
				if !r.Remove(name) {
					return notFoundErr(fmt.Errorf("no registered NAS named %q", args[0]))
				}
				if keepData {
					return nil
				}
				p, err := fleetDBPath(dbPath)
				if err != nil {
					return err
				}
				if _, statErr := os.Stat(p); statErr != nil {
					return nil
				}
				s, err := openFleetWrite(cmd.Context(), p)
				if err != nil {
					return err
				}
				defer s.Close()
				if err := fleet.PurgeNAS(cmd.Context(), s.DB(), name); err != nil {
					return err
				}
				purged = true
				return nil
			}); err != nil {
				var ce *cliError
				if As(err, &ce) {
					return err
				}
				return configErr(err)
			}
			return printJSONFiltered(cmd.OutOrStdout(), map[string]any{"removed": name, "mirror_purged": purged}, flags)
		},
	}
	cmd.Flags().BoolVar(&keepData, "keep-data", false, "Keep the NAS's rows in the local mirror")
	cmd.Flags().StringVar(&dbPath, "db", "", "Fleet mirror path (default <data dir>/fleet.db)")
	return cmd
}
