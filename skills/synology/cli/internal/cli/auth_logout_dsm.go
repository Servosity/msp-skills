// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// HAND-AUTHORED. auth logout: forget what this computer holds for a NAS (or
// for every NAS), and report exactly what was removed.

package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/spf13/cobra"

	"synology-pp-cli/internal/cliutil"
	"synology-pp-cli/internal/config"
	"synology-pp-cli/internal/dsm"
)

const authLogoutLong = `Forgets, on this computer, the cached DSM session for one NAS (or, with
--all, for every NAS): the whole session file, which holds every account's
session id, SynoToken and session cookies and every two-factor trusted-device
token kept for that NAS, plus the login stored in the CLI's own config
(account, password, device_id, otp_code or auth_header fields in config.json,
and the credentials file). It prints every file it removed or changed, or that
nothing was cached.

The NAS is the one SYNOLOGY_BASE_URL (or the config's base_url) names, or the
registered NAS passed with --nas. Without either it refuses: pass --all to
forget every cached session in the data directory (the single NAS and every
fleet NAS alike).

Local only: the NAS is not contacted, so a session id stays valid on the NAS
until DSM times it out; run 'synology-cli session logout' first to end it there
too. After auth logout, the next command for that NAS logs in again with the
password, and a two-factor account needs a fresh one-time code (unless
SYNOLOGY_DEVICE_ID, or <prefix>_DEVICE_ID for a fleet NAS, supplies a token).
Environment variables are not changed; the output names those still set.`

// configCredentialKeys are the config.json fields that carry a login.
var configCredentialKeys = []string{"auth_header", "access_token", "refresh_token", "token_expiry",
	"client_id", "client_secret", "account", "password", "device_id", "otp_code"}

// logoutCredentialFile is one place the CLI's own config stores a login.
type logoutCredentialFile struct {
	File string `json:"file"`
	// Kind: "config" (a config file whose credential fields are removed; the
	// rest of the file is kept) or "credentials" (the credentials file,
	// removed whole).
	Kind   string   `json:"kind"`
	Fields []string `json:"fields,omitempty"`
}

// logoutReport is auth logout's answer: what it removed (or, with
// --dry-run, would remove). Nothing is listed that was not acted on.
type logoutReport struct {
	DryRun bool `json:"dry_run,omitempty"`
	// Scope: "nas" (one NAS) or "all" (every cached session).
	Scope string `json:"scope"`
	NAS   string `json:"nas,omitempty"`
	// SessionFiles are the session cache files removed (would be removed).
	SessionFiles []dsm.CachedSession `json:"session_files"`
	// StoredCredentials are the config and credentials files cleared.
	StoredCredentials []logoutCredentialFile `json:"stored_credentials"`
	// Cleared is true only when something was removed or changed.
	Cleared      bool     `json:"cleared"`
	NASContacted bool     `json:"nas_contacted"`
	Result       string   `json:"result"`
	EnvStillSet  []string `json:"env_still_set"`
	Note         string   `json:"note,omitempty"`
}

func (r logoutReport) ReportIdentity() string { return r.Result }

// runAuthLogout implements the auth logout contract. HAND-FIX(dsm-auth-logout).
func runAuthLogout(cmd *cobra.Command, flags *rootFlags, all bool, nasName string) error {
	if all && nasName != "" {
		return usageErr(errors.New("--all and --nas are mutually exclusive; nothing was cleared"))
	}
	cfg, err := config.Load(flags.configPath)
	if err != nil {
		return configErr(err)
	}
	rep := logoutReport{DryRun: dryRunOK(flags), Scope: "all", SessionFiles: []dsm.CachedSession{},
		StoredCredentials: []logoutCredentialFile{}, EnvStillSet: []string{}}
	target, envPrefixes := "", []string{"SYNOLOGY"}
	switch {
	case all:
		if list, _, _, ferr := fleetNAS(flags); ferr == nil {
			for _, n := range list {
				if n.EnvPrefix != "SYNOLOGY" {
					envPrefixes = append(envPrefixes, n.EnvPrefix)
				}
			}
		}
	case nasName != "":
		n, rerr := resolveNAS(flags, strings.ToLower(nasName))
		if rerr != nil {
			return rerr
		}
		target, envPrefixes = n.URL, []string{n.EnvPrefix}
	default:
		target = strings.TrimSpace(cfg.BaseURL)
		if dsm.SessionKey(target) == "" || dsm.PlaceholderOrigin(target) {
			if rep.DryRun && requiredInputWaived() {
				return writeDryRun(cmd.OutOrStdout(), flags, "auth logout (no NAS address: refused without --all or --nas)")
			}
			why := "SYNOLOGY_BASE_URL is not set (the address is the example placeholder " + dsm.DisplayAddress(target) + ")"
			if dsm.SessionKey(target) == "" {
				why = "SYNOLOGY_BASE_URL is not a usable NAS address"
			}
			return usageErr(fmt.Errorf("auth logout: %s, so there is no NAS to log out of; set SYNOLOGY_BASE_URL to the NAS whose cached session to forget (for example https://<nas>:5001), pass --nas <name> for a registered NAS, or run 'synology-cli auth logout --all' to forget every cached session (single NAS and fleet NAS) and the stored login; nothing was cleared", why))
		}
	}
	if target != "" {
		rep.Scope, rep.NAS = "nas", strings.TrimRight(dsm.DisplayAddress(target), "/")
	}

	dataDir, err := cliutil.DataDir()
	if err != nil {
		return configErr(fmt.Errorf("locating the data directory: %w", err))
	}
	// Early builds kept an origin-less session.json next to the config file.
	dirs := []string{dataDir}
	if d := filepath.Dir(cfg.Path); cfg.Path != "" && d != dataDir {
		dirs = append(dirs, d)
	}
	files, err := dsm.CachedSessions(target, dirs...)
	if err != nil {
		return configErr(err)
	}
	creds := storedLogins(cfg, flags.configPath != "")

	if rep.DryRun {
		rep.SessionFiles, rep.StoredCredentials = files, creds
	} else {
		removed, ferr := dsm.ForgetSessionFiles(files)
		rep.SessionFiles = removed
		cleared, cerr := clearStoredLogins(creds)
		rep.StoredCredentials = cleared
		if err := errors.Join(ferr, cerr); err != nil {
			rep.Cleared = len(removed) > 0 || len(cleared) > 0
			return configErr(fmt.Errorf("auth logout stopped: %w (removed before that: %s)", err, rep.removedSummary()))
		}
		rep.Cleared = len(removed) > 0 || len(cleared) > 0
	}
	rep.EnvStillSet = envStillSet(envPrefixes)
	if len(rep.EnvStillSet) > 0 {
		rep.Note = "still set in this environment (a later command can log in with them): " + strings.Join(rep.EnvStillSet, ", ")
	}
	rep.Result = rep.headline()

	w := cmd.OutOrStdout()
	if flags.asJSON || flags.selectFields != "" || flags.compact {
		return printReportJSON(w, rep, flags)
	}
	return rep.writeHuman(w)
}

func (r logoutReport) where() string {
	if r.Scope == "all" {
		return "every NAS"
	}
	return r.NAS
}

func (r logoutReport) headline() string {
	n := len(r.SessionFiles) + len(r.StoredCredentials)
	switch {
	case r.DryRun && n == 0:
		return fmt.Sprintf("dry-run: nothing cached for %s on this computer; nothing would be removed", r.where())
	case r.DryRun:
		return fmt.Sprintf("dry-run: would forget %d session file(s) and clear %d stored login file(s) for %s on this computer (local only: the NAS would not be contacted); nothing was changed", len(r.SessionFiles), len(r.StoredCredentials), r.where())
	case n == 0:
		return fmt.Sprintf("nothing cached for %s on this computer (no session file, no stored login); nothing was removed", r.where())
	}
	return fmt.Sprintf("forgot %d session file(s) and cleared %d stored login file(s) for %s on this computer (local only: the NAS was not contacted)", len(r.SessionFiles), len(r.StoredCredentials), r.where())
}

func (r logoutReport) removedSummary() string {
	var parts []string
	for _, f := range r.SessionFiles {
		parts = append(parts, f.File)
	}
	for _, c := range r.StoredCredentials {
		parts = append(parts, c.File)
	}
	if len(parts) == 0 {
		return "nothing"
	}
	return strings.Join(parts, ", ")
}

func (r logoutReport) writeHuman(w io.Writer) error {
	verb, clear := "removed", "cleared"
	if r.DryRun {
		verb, clear = "would remove", "would clear"
	}
	var b strings.Builder
	line := func(format string, a ...any) { // each line scrubbed on its own: the newline is ours
		b.WriteString(cliutil.ScrubTerminal(fmt.Sprintf(format, a...)) + "\n")
	}
	line("%s", r.Result)
	for _, f := range r.SessionFiles {
		var held []string
		if f.SessionID {
			held = append(held, "session id")
		}
		if len(f.DeviceTokenAccounts) > 0 {
			held = append(held, "trusted-device token for "+strings.Join(f.DeviceTokenAccounts, ", "))
		}
		if len(held) == 0 {
			held = append(held, "no session")
		}
		nas := f.NAS
		if nas == "" {
			nas = "no NAS recorded"
		}
		line("  %s %s (%s: %s)", verb, f.File, nas, strings.Join(held, "; "))
	}
	for _, c := range r.StoredCredentials {
		if c.Kind == "credentials" {
			line("  %s %s (stored login file)", verb, c.File)
			continue
		}
		line("  %s %s in %s (the rest of the file is kept)", clear, strings.Join(c.Fields, ", "), c.File)
	}
	if r.Note != "" {
		line("Note: %s", r.Note)
	}
	_, err := io.WriteString(w, b.String())
	return err
}

// storedLogins lists the CLI's own config and credentials files that hold a
// login, in the places config.Load reads them from.
func storedLogins(cfg *config.Config, explicitConfig bool) []logoutCredentialFile {
	out := []logoutCredentialFile{}
	addConfig := func(path string) {
		if fields := configCredentialFields(path); len(fields) > 0 {
			out = append(out, logoutCredentialFile{File: path, Kind: "config", Fields: fields})
		}
	}
	addCredentials := func(path string, err error) {
		if err != nil || path == "" {
			return
		}
		for _, c := range out {
			if c.File == path {
				return
			}
		}
		if fi, serr := os.Stat(path); serr == nil && !fi.IsDir() {
			out = append(out, logoutCredentialFile{File: path, Kind: "credentials"})
		}
	}
	if cfg.Path != "" {
		addConfig(cfg.Path)
		if _, err := os.Stat(cfg.Path); errors.Is(err, os.ErrNotExist) && !explicitConfig {
			if legacy, lerr := config.LegacyConfigPath(); lerr == nil && legacy != cfg.Path {
				addConfig(legacy) // config.Load falls back to it while the config file is absent
			}
		}
		if explicitConfig {
			addCredentials(cliutil.CredentialsFilePathForConfig(cfg.Path))
		}
	}
	addCredentials(cliutil.CredentialsFilePath())
	return out
}

// configCredentialFields names the credential fields set in a config file.
func configCredentialFields(path string) []string {
	m, ok := readConfigMap(path)
	if !ok {
		return nil
	}
	var fields []string
	for _, k := range configCredentialKeys {
		if k == "token_expiry" {
			continue // a timestamp, not a credential; removed with the token
		}
		if v, _ := m[k].(string); v != "" {
			fields = append(fields, k)
		}
	}
	return fields
}

func readConfigMap(path string) (map[string]any, bool) {
	data, err := os.ReadFile(filepath.Clean(path)) // the CLI's own config path (config.Load's resolution or --config)
	if err != nil {
		return nil, false
	}
	m := map[string]any{}
	if json.Unmarshal(data, &m) != nil {
		return nil, false
	}
	return m, true
}

// clearStoredLogins removes the credential fields from each config file (the
// rest of the file is kept as it was) and removes each credentials file. It
// returns what it cleared; the first failure stops it.
func clearStoredLogins(files []logoutCredentialFile) ([]logoutCredentialFile, error) {
	cleared := make([]logoutCredentialFile, 0, len(files))
	for _, f := range files {
		if f.Kind == "credentials" {
			err := os.Remove(f.File)
			if errors.Is(err, os.ErrNotExist) {
				continue
			}
			if err != nil {
				return cleared, fmt.Errorf("removing %s: %w", f.File, err)
			}
			cleared = append(cleared, f)
			continue
		}
		m, ok := readConfigMap(f.File)
		if !ok {
			return cleared, fmt.Errorf("reading %s: not a readable JSON config", f.File)
		}
		for _, k := range configCredentialKeys {
			delete(m, k)
		}
		data, err := json.MarshalIndent(m, "", "  ")
		if err != nil {
			return cleared, err
		}
		if err := cliutil.AtomicWritePrivateFile(f.File, data, 0o600, 0o700); err != nil {
			return cleared, fmt.Errorf("writing %s: %w", f.File, err)
		}
		if left := configCredentialFields(f.File); len(left) > 0 {
			return cleared, fmt.Errorf("%s still holds %s after clearing", f.File, strings.Join(left, ", "))
		}
		cleared = append(cleared, f)
	}
	return cleared, nil
}

// envStillSet names the login env vars still set for these prefixes.
func envStillSet(prefixes []string) []string {
	out := []string{}
	seen := map[string]bool{}
	for _, p := range prefixes {
		for _, suffix := range []string{"_ACCOUNT", "_PASSWORD", "_DEVICE_ID", "_OTP_CODE"} {
			name := p + suffix
			if !seen[name] && os.Getenv(name) != "" {
				seen[name] = true
				out = append(out, name)
			}
		}
	}
	sort.Strings(out)
	return out
}
