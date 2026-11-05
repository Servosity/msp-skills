// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// pp:data-source local
// Client-account profiles for MSP fleet mode: one Zoom Server-to-Server OAuth
// app (account ID + client ID + client secret) per client Zoom account.

package cli

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/pelletier/go-toml/v2"
	"github.com/spf13/cobra"

	"zoom-phone-pp-cli/internal/cliutil"
	"zoom-phone-pp-cli/internal/config"
)

// Layout. Each account is its own CLI home so nothing is shared between
// tenants:
//
//	<config dir>/accounts/<name>/config.toml             client_id, client_secret, phone_account_id
//	<config dir>/accounts/<name>/data/credentials.toml   written by the token mint
//	<config dir>/accounts/<name>/data/data-*.db          the account's local store
//
// The generated token mint (config.SaveTokens) does NOT write the token back
// into the config file. It moves every credential field into
// <DataDir>/credentials.toml and rewrites the config file without them. A run
// that sets only ZOOM_PHONE_CONFIG would therefore push the account's secret
// into the operator's default credentials file, and the next run of a
// different account would silently read it. Running each account with
// ZOOM_PHONE_HOME=<account dir> as well keeps DataDir (and the store, state and
// cache) inside the account, and config.Load's per-config credentials lookup
// (<dir of config>/data/credentials.toml) resolves to the same file. The client
// hook in zoom_accounts_guard.go refuses API use when that pairing is broken.
const (
	acctConfigFileName  = "config.toml"
	acctDirName         = "accounts"
	acctEnvPrefix       = "ZOOM_PHONE_"
	acctDefaultBaseURL  = "https://api.zoom.us/v2"
	acctDefaultTokenURL = "https://zoom.us/oauth/token" // #nosec G101 -- public OAuth token endpoint URL, not a credential
)

var acctNameRe = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{0,62}$`)

var acctShellNameRe = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

// acctCredentialEnvVars override any config file, so a process that must use
// one account's file has to run without them.
var acctCredentialEnvVars = []string{
	"ZOOM_PHONE_ACCOUNT_ID",
	"ZOOM_PHONE_CLIENT_ID",
	"ZOOM_PHONE_CLIENT_SECRET",
}

// acctPathEnvVars choose where config, data, state and cache live. The
// per-kind variables outrank ZOOM_PHONE_HOME.
var acctPathEnvVars = []string{
	"ZOOM_PHONE_CONFIG",
	"ZOOM_PHONE_HOME",
	"ZOOM_PHONE_CONFIG_DIR",
	"ZOOM_PHONE_DATA_DIR",
	"ZOOM_PHONE_STATE_DIR",
	"ZOOM_PHONE_CACHE_DIR",
}

// acctHostEnvVars redirect where the Basic-auth token request and bearer calls
// go (and generated code persists them into the config file), so they never
// follow an operator's shell into a per-account run.
var acctHostEnvVars = []string{
	"ZOOM_PHONE_BASE_URL",
	"ZOOM_PHONE_TOKEN_URL",
	"ZOOM_PHONE_OAUTH_SCOPE",
}

// acctEnvAllowlist is the complete set of ZOOM_PHONE_* variables a per-account
// process inherits. Every other ZOOM_PHONE_* variable (credentials, paths,
// hosts, feedback endpoints, MCP tokens, anything added later) is dropped.
var acctEnvAllowlist = map[string]bool{
	"ZOOM_PHONE_NO_LEARN":         true,
	"ZOOM_PHONE_LEARN_SESSION":    true,
	"ZOOM_PHONE_LEARN_NO_CAPTURE": true,
	"ZOOM_PHONE_LEARN_SURFACE":    true,
	"ZOOM_PHONE_NO_AUTO_REFRESH":  true,
	"ZOOM_PHONE_USER_AGENT":       true,
	"ZOOM_PHONE_MCP_READ_ONLY":    true,
}

// acctEnvKeyDropped reports whether a variable must not reach a per-account
// process.
func acctEnvKeyDropped(key string) bool {
	upper := strings.ToUpper(key)
	return strings.HasPrefix(upper, acctEnvPrefix) && !acctEnvAllowlist[upper]
}

// acctEnvUnsetNames lists what `accounts path --env` unsets: the known
// credential, path and host variables plus every non-allowlisted ZOOM_PHONE_*
// variable exported in environ. ZOOM_PHONE_CONFIG and ZOOM_PHONE_HOME are
// exported right after, so they are not listed.
func acctEnvUnsetNames(environ []string) []string {
	set := map[string]bool{}
	for _, group := range [][]string{acctCredentialEnvVars, acctPathEnvVars, acctHostEnvVars} {
		for _, k := range group {
			set[k] = true
		}
	}
	for _, kv := range environ {
		key, _, _ := strings.Cut(kv, "=")
		if acctEnvKeyDropped(key) && acctShellNameRe.MatchString(key) {
			set[key] = true
		}
	}
	delete(set, "ZOOM_PHONE_CONFIG")
	delete(set, "ZOOM_PHONE_HOME")
	out := make([]string, 0, len(set))
	for k := range set {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func acctValidateName(name string) error {
	if !acctNameRe.MatchString(name) {
		return fmt.Errorf("invalid account name %q: use 1-63 lowercase letters, digits, '-' or '_', starting with a letter or digit", name)
	}
	return nil
}

// acctLooksLikeAccountHome reports whether dir is (or was) an account home:
// <...>/accounts/<valid name> holding config.toml directly, or not existing.
// A general-purpose ZOOM_PHONE_HOME keeps its config under config/, so it
// does not match.
func acctLooksLikeAccountHome(dir string) bool {
	dir = filepath.Clean(dir)
	if filepath.Base(filepath.Dir(dir)) != acctDirName || acctValidateName(filepath.Base(dir)) != nil {
		return false
	}
	if st, err := os.Lstat(filepath.Join(dir, acctConfigFileName)); err == nil {
		return st.Mode().IsRegular()
	}
	_, err := os.Lstat(dir)
	return errors.Is(err, fs.ErrNotExist)
}

// acctRootDir resolves <config dir>/accounts for the operator, not for the
// currently selected account: when ZOOM_PHONE_HOME points at an account home,
// the accounts directory is its parent, so `accounts list` and `fleet` still
// see every account after `eval "$(... accounts path acme --env)"`. Explicit
// ZOOM_PHONE_CONFIG_DIR and --home still win; ZOOM_PHONE_CONFIG never moves it.
func acctRootDir() (string, error) {
	res, err := cliutil.ResolveKindDir(cliutil.PathKindConfig)
	if err != nil {
		return "", err
	}
	if res.Rung == "home-env" {
		if home, ok := cliutil.CleanPathOverride(os.Getenv("ZOOM_PHONE_HOME")); ok && acctLooksLikeAccountHome(home) {
			return filepath.Dir(home), nil
		}
	}
	return filepath.Join(res.Dir, acctDirName), nil
}

func acctHomeDir(name string) (string, error) {
	if err := acctValidateName(name); err != nil {
		return "", err
	}
	root, err := acctRootDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(root, name), nil
}

func acctConfigPath(name string) (string, error) {
	home, err := acctHomeDir(name)
	if err != nil {
		return "", err
	}
	return filepath.Join(home, acctConfigFileName), nil
}

// acctRefuseSymlinkDir refuses a symlinked (or non-directory) accounts or
// account directory, so a profile write or delete never lands elsewhere.
func acctRefuseSymlinkDir(path string) error {
	st, err := os.Lstat(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil
		}
		return err
	}
	if st.Mode()&os.ModeSymlink != 0 {
		return configErr(fmt.Errorf("refusing to use %s: it is a symlink; account profiles must live in a real directory", path))
	}
	if !st.IsDir() {
		return configErr(fmt.Errorf("refusing to use %s: it is not a directory", path))
	}
	return nil
}

// acctFileFields is the subset of the config and credentials TOML that the
// accounts commands report on. Unknown keys are ignored.
type acctFileFields struct {
	ClientID     string    `toml:"client_id"`
	ClientSecret string    `toml:"client_secret"`
	AccountID    string    `toml:"phone_account_id"`
	AccessToken  string    `toml:"access_token"`
	TokenExpiry  time.Time `toml:"token_expiry"`
	BaseURL      string    `toml:"base_url"`
	TokenURL     string    `toml:"token_url"`
}

// acctInfo is the non-secret view of one saved account. It never carries the
// client secret or an access token.
type acctInfo struct {
	Name        string `json:"name"`
	Path        string `json:"path"`
	AccountID   string `json:"account_id"`
	HasClientID bool   `json:"has_client_id"` // credkit-wiring: half of the Basic credential, held, so never listed
	HasSecret   bool   `json:"has_secret"`
	TokenCached bool   `json:"token_cached"`
	BaseURL     string `json:"base_url,omitempty"`
	TokenURL    string `json:"token_url,omitempty"`
	Error       string `json:"error,omitempty"`

	home string
}

var acctInfoKeys = []string{"name", "path", "account_id", "has_client_id", "has_secret", "token_cached", "base_url", "token_url", "error"}

func acctReadFields(path string) (acctFileFields, bool, error) {
	var out acctFileFields
	data, err := os.ReadFile(filepath.Clean(path)) // #nosec G304 -- path is built from a validated account name under the CLI config dir.
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return out, false, nil
		}
		return out, false, err
	}
	cliutil.HoldConfigFile(data) // credkit-wiring: a profile is a credential source, held before its parse or any output
	if err := toml.Unmarshal(data, &out); err != nil {
		return out, true, fmt.Errorf("parsing %s: %w", path, err)
	}
	return out, true, nil
}

// acctNonDefaultURL returns v when it is set and differs from the built-in
// default, else "".
func acctNonDefaultURL(v, def string) string {
	v = strings.TrimSpace(v)
	if v == "" || strings.TrimRight(v, "/") == strings.TrimRight(def, "/") {
		return ""
	}
	return v
}

// acctLoadInfo merges the account's config file with the credentials file the
// token mint writes, the same precedence config.Load uses (config file first,
// credentials file fills the gaps).
func acctLoadInfo(name string, now time.Time) (acctInfo, error) {
	home, err := acctHomeDir(name)
	if err != nil {
		return acctInfo{}, err
	}
	path := filepath.Join(home, acctConfigFileName)
	info := acctInfo{Name: name, Path: path, home: home}
	// Never read through a tampered tree: it could show another account's app.
	if err := acctVerifyStorageTree(name, home); err != nil {
		info.Error = err.Error()
		return info, nil
	}
	cfg, _, cfgErr := acctReadFields(path)
	merged := cfg
	var errs []string
	if cfgErr != nil {
		errs = append(errs, cfgErr.Error())
	}
	if credsPath, err := cliutil.CredentialsFilePathForConfig(path); err == nil {
		creds, _, credsErr := acctReadFields(credsPath)
		if credsErr != nil {
			errs = append(errs, credsErr.Error())
		}
		if merged.ClientID == "" {
			merged.ClientID = creds.ClientID
		}
		if merged.ClientSecret == "" {
			merged.ClientSecret = creds.ClientSecret
		}
		if merged.AccountID == "" {
			merged.AccountID = creds.AccountID
		}
		if merged.AccessToken == "" {
			merged.AccessToken = creds.AccessToken
			merged.TokenExpiry = creds.TokenExpiry
		}
	}
	info.AccountID = merged.AccountID
	info.HasClientID = merged.ClientID != ""
	info.HasSecret = merged.ClientSecret != ""
	info.TokenCached = merged.AccessToken != "" && (merged.TokenExpiry.IsZero() || merged.TokenExpiry.After(now))
	info.BaseURL = acctNonDefaultURL(cfg.BaseURL, acctDefaultBaseURL)
	info.TokenURL = acctNonDefaultURL(cfg.TokenURL, acctDefaultTokenURL)
	info.Error = strings.Join(errs, "; ")
	return info, nil
}

// acctList returns every saved account sorted by name. A missing accounts
// directory is an empty list, not an error.
func acctList() ([]acctInfo, error) {
	root, err := acctRootDir()
	if err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return make([]acctInfo, 0), nil
		}
		return nil, err
	}
	now := time.Now()
	out := make([]acctInfo, 0, len(entries))
	for _, entry := range entries {
		name := entry.Name()
		if !entry.IsDir() || acctValidateName(name) != nil {
			continue
		}
		st, err := os.Lstat(filepath.Join(root, name, acctConfigFileName))
		if err != nil || !st.Mode().IsRegular() {
			continue
		}
		info, err := acctLoadInfo(name, now)
		if err != nil {
			continue
		}
		out = append(out, info)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

// acctExists reports whether a saved account's config file exists.
func acctExists(name string) (bool, error) {
	path, err := acctConfigPath(name)
	if err != nil {
		return false, err
	}
	st, err := os.Lstat(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return false, nil
		}
		return false, err
	}
	return st.Mode().IsRegular(), nil
}

// acctShellQuote single-quotes s for POSIX shells.
func acctShellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

func init() {
	registerNovelCommand(func(root *cobra.Command, flags *rootFlags) {
		parent, _, err := root.Find([]string{"accounts"})
		if err != nil || parent == nil || parent == root {
			return
		}
		addNovelCommandIfAbsent(parent, newAcctListCmd(flags))
		addNovelCommandIfAbsent(parent, newAcctRemoveCmd(flags))
		addNovelCommandIfAbsent(parent, newAcctPathCmd(flags))
	})
}

func newAcctListCmd(flags *rootFlags) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List saved client-account profiles (never prints secrets or tokens)",
		Long: `List every client-account profile saved with 'accounts connect', sorted by name.

Each row shows the profile name, its config file path, the Zoom account ID, the
client ID, whether a client secret is stored and whether an unexpired access
token is cached. base_url and token_url appear only when a profile points at a
non-default host. The client secret and the token are never printed.`,
		Example: strings.Trim(`
  zoom-phone-cli accounts list
  zoom-phone-cli accounts list --json`, "\n"),
		Annotations: map[string]string{"mcp:read-only": "true", "pp:data-source": "local"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return writeDryRun(cmd.OutOrStdout(), flags, "accounts list")
			}
			accounts, err := acctList()
			if err != nil {
				return err
			}
			if !wantsHumanTable(cmd.OutOrStdout(), flags) {
				return printJSONFilteredKeep(cmd.OutOrStdout(), accounts, flags, acctInfoKeys...)
			}
			if len(accounts) == 0 {
				fmt.Fprintln(cmd.ErrOrStderr(), "no saved accounts; add one with: zoom-phone-cli accounts connect <name> --account-id <id> --client-id <id> --client-secret-stdin")
				return nil
			}
			overrides := false
			for _, a := range accounts {
				if a.BaseURL != "" || a.TokenURL != "" {
					overrides = true
				}
			}
			tw := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 4, 2, ' ', 0)
			header := "NAME\tACCOUNT_ID\tCLIENT\tSECRET\tTOKEN\tPATH"
			if overrides {
				header += "\tHOSTS"
			}
			fmt.Fprintln(tw, header)
			for _, a := range accounts {
				row := fmt.Sprintf("%s\t%s\t%t\t%t\t%t\t%s", a.Name, a.AccountID, a.HasClientID, a.HasSecret, a.TokenCached, a.Path)
				if overrides {
					var hosts []string
					if a.BaseURL != "" {
						hosts = append(hosts, "base_url="+a.BaseURL)
					}
					if a.TokenURL != "" {
						hosts = append(hosts, "token_url="+a.TokenURL)
					}
					row += "\t" + strings.Join(hosts, " ")
				}
				fmt.Fprintln(tw, row)
			}
			return tw.Flush()
		},
	}
	return cmd
}

func newAcctRemoveCmd(flags *rootFlags) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "remove [name]",
		Short: "Delete a saved client-account profile, its cached token and its local store",
		Long: `Delete one client-account profile saved with 'accounts connect'.

This removes the whole account directory: the config file holding the client
ID and secret, the cached access token and the account's local store.`,
		Example: strings.Trim(`
  zoom-phone-cli accounts remove acme
  zoom-phone-cli accounts remove acme --dry-run`, "\n"),
		// mcp:hidden: deleting a credential profile is not an agent tool.
		Annotations: map[string]string{"mcp:hidden": "true", "pp:data-source": "local", "pp:happy-args": "name=nonexistent-account", "pp:typed-exit-codes": "0,2,3"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 && cmd.Flags().NFlag() == 0 {
				return cmd.Help()
			}
			if dryRunOK(flags) {
				return writeDryRun(cmd.OutOrStdout(), flags, "accounts remove")
			}
			if len(args) == 0 {
				_ = cmd.Usage()
				return usageErr(errors.New("account name is required"))
			}
			if len(args) > 1 {
				_ = cmd.Usage()
				return usageErr(fmt.Errorf("accounts remove takes one name, got %d", len(args)))
			}
			name := args[0]
			if err := acctValidateName(name); err != nil {
				return usageErr(err)
			}
			home, err := acctHomeDir(name)
			if err != nil {
				return usageErr(err)
			}
			if err := acctRefuseSymlinkDir(filepath.Dir(home)); err != nil {
				return err
			}
			// Take the profile lock first (it lives in <accounts>/.locks, outside the
			// tree being removed), so a concurrent token save or connect waits and
			// the lock survives the removal.
			release, lockErr := config.ZoomAcquireProfileLock(filepath.Join(home, acctConfigFileName))
			if lockErr != nil {
				return configErr(fmt.Errorf("locking account %q: %w", name, lockErr))
			}
			defer release()
			if _, err := os.Lstat(home); err != nil {
				if errors.Is(err, fs.ErrNotExist) {
					return notFoundErr(fmt.Errorf("account %q not found; run 'zoom-phone-cli accounts list' to see saved accounts", name))
				}
				return err
			}
			if err := acctRefuseSymlinkDir(home); err != nil {
				return err
			}
			if err := acctVerifyStorageTree(name, home); err != nil {
				return err
			}
			if err := os.RemoveAll(home); err != nil {
				return fmt.Errorf("removing account %q: %w", name, err)
			}
			result := map[string]any{"name": name, "path": filepath.Join(home, acctConfigFileName), "removed": true}
			if !wantsHumanTable(cmd.OutOrStdout(), flags) {
				return printJSONFilteredKeep(cmd.OutOrStdout(), result, flags, "name", "path", "removed")
			}
			fmt.Fprintf(cmd.OutOrStdout(), "removed account %s (%s)\n", name, home)
			return nil
		},
	}
	return cmd
}

func newAcctPathCmd(flags *rootFlags) *cobra.Command {
	var flagEnv bool
	cmd := &cobra.Command{
		Use:   "path [name]",
		Short: "Print a saved account's config file path, or the shell exports that select it",
		Long: `Print the config file path of one saved client-account profile.

To run commands against that account directly, use --env: it prints shell lines
that unset the credential, path and host override variables (including
ZOOM_PHONE_BASE_URL and ZOOM_PHONE_TOKEN_URL) and export BOTH ZOOM_PHONE_CONFIG
and ZOOM_PHONE_HOME. Setting ZOOM_PHONE_CONFIG alone is refused at the first API
call: a token mint would move the account's credentials out of its profile. To
run one command across accounts, use 'fleet' instead.`,
		Example: strings.Trim(`
  zoom-phone-cli accounts path acme
  eval "$(zoom-phone-cli accounts path acme --env)" && zoom-phone-cli users list-phone
  zoom-phone-cli fleet --accounts acme -- users list-phone`, "\n"),
		Annotations: map[string]string{"mcp:read-only": "true", "pp:data-source": "local", "pp:happy-args": "name=acme", "pp:typed-exit-codes": "0,2"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 && cmd.Flags().NFlag() == 0 {
				return cmd.Help()
			}
			if dryRunOK(flags) {
				return writeDryRun(cmd.OutOrStdout(), flags, "accounts path")
			}
			if len(args) != 1 {
				_ = cmd.Usage()
				return usageErr(errors.New("exactly one account name is required"))
			}
			name := args[0]
			home, err := acctHomeDir(name)
			if err != nil {
				return usageErr(err)
			}
			path := filepath.Join(home, acctConfigFileName)
			exists, err := acctExists(name)
			if err != nil {
				return err
			}
			if !exists {
				fmt.Fprintf(cmd.ErrOrStderr(), "warning: account %q is not saved yet; add it with: zoom-phone-cli accounts connect %s --account-id <id> --client-id <id> --client-secret-stdin\n", name, name)
			}
			if flagEnv {
				w := cmd.OutOrStdout()
				fmt.Fprintf(w, "unset %s\n", strings.Join(acctEnvUnsetNames(os.Environ()), " "))
				fmt.Fprintf(w, "export ZOOM_PHONE_CONFIG=%s\n", acctShellQuote(path))
				fmt.Fprintf(w, "export ZOOM_PHONE_HOME=%s\n", acctShellQuote(home))
				return nil
			}
			if flags.asJSON || flags.agent || flags.selectFields != "" {
				return printJSONFilteredKeep(cmd.OutOrStdout(), map[string]any{"name": name, "path": path, "home": home}, flags, "name", "path", "home")
			}
			fmt.Fprintln(cmd.OutOrStdout(), path)
			return nil
		},
	}
	cmd.Flags().BoolVar(&flagEnv, "env", false, "Print POSIX shell lines (unset overrides, export ZOOM_PHONE_CONFIG and ZOOM_PHONE_HOME) that select this account")
	return cmd
}
