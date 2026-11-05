// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// pp:data-source local
// Regen-durable isolation guard for client-account profiles (see the layout
// note in zoom_accounts.go).

package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"zoom-phone-pp-cli/internal/client"
	"zoom-phone-pp-cli/internal/cliutil"
)

// An account profile is only safe to use when the process's data directory is
// that account's own <account dir>/data, and an account's data directory is
// only safe to use with that account's profile. Otherwise the generated token
// mint moves credentials between the profile and some other credentials file
// (the operator's default one, or another account's), and the next run of
// either authenticates as the wrong tenant.
func init() {
	registerClientHook(acctClientGuard)
	// auth login / set-token / logout write or clear credentials without going
	// through newClient, so they get the same check.
	registerNovelCommand(func(root *cobra.Command, flags *rootFlags) {
		auth, _, err := root.Find([]string{"auth"})
		if err != nil || auth == nil || auth == root {
			return
		}
		for _, sub := range auth.Commands() {
			if sub.Name() == "status" || sub.Name() == "help" || sub.RunE == nil {
				continue
			}
			inner := sub.RunE
			sub.RunE = func(cmd *cobra.Command, args []string) error {
				if err := acctCheckIsolation(acctResolvedConfigPath(flags)); err != nil {
					return err
				}
				return inner(cmd, args)
			}
		}
	})
}

func acctClientGuard(c *client.Client) error {
	if c == nil || c.Config == nil {
		return nil
	}
	return acctCheckIsolation(c.Config.Path)
}

// acctResolvedConfigPath mirrors config.resolveConfigPath: --config, then
// ZOOM_PHONE_CONFIG, then <config dir>/config.toml.
func acctResolvedConfigPath(flags *rootFlags) string {
	if flags != nil {
		if p := strings.TrimSpace(flags.configPath); p != "" {
			return p
		}
	}
	if p := os.Getenv("ZOOM_PHONE_CONFIG"); p != "" {
		return p
	}
	dir, err := cliutil.ConfigDir()
	if err != nil {
		return ""
	}
	return filepath.Join(dir, acctConfigFileName)
}

// acctCanonicalPath resolves symlinks on the deepest existing ancestor so two
// spellings of one location compare equal (macOS /var vs /private/var).
func acctCanonicalPath(path string) string {
	if path == "" {
		return ""
	}
	path = filepath.Clean(path)
	if abs, err := filepath.Abs(path); err == nil {
		path = abs
	}
	remainder := ""
	for {
		if resolved, err := filepath.EvalSymlinks(path); err == nil {
			return filepath.Join(resolved, remainder)
		}
		parent := filepath.Dir(path)
		if parent == path {
			return filepath.Join(path, remainder)
		}
		remainder = filepath.Join(filepath.Base(path), remainder)
		path = parent
	}
}

func acctSamePath(a, b string) bool {
	return acctCanonicalPath(a) == acctCanonicalPath(b)
}

// acctSameDir compares two directories by file identity when both exist (so
// case variants on a case-insensitive file system and symlinks match), else
// by canonical path, case-insensitively.
func acctSameDir(a, b string) bool {
	sa, errA := os.Stat(a)
	sb, errB := os.Stat(b)
	if errA == nil && errB == nil {
		return os.SameFile(sa, sb)
	}
	return strings.EqualFold(acctCanonicalPath(a), acctCanonicalPath(b))
}

// acctPathWithin reports whether child is parent or below it, comparing
// canonical paths case-insensitively (a false match only refuses a run).
func acctPathWithin(child, parent string) bool {
	c := strings.ToLower(acctCanonicalPath(child))
	p := strings.ToLower(acctCanonicalPath(parent))
	if c == "" || p == "" {
		return false
	}
	return c == p || strings.HasPrefix(c, strings.TrimSuffix(p, string(filepath.Separator))+string(filepath.Separator))
}

// acctShapeHome recognises <...>/accounts/<name>/config.toml by path shape,
// case-insensitively, and returns the account directory as spelled. The app's
// own config directory name is excluded so XDG_CONFIG_HOME=<...>/accounts does
// not turn the default config into a "profile".
func acctShapeHome(cfgPath string) (name, home string, ok bool) {
	p := filepath.Clean(cfgPath)
	if abs, err := filepath.Abs(p); err == nil {
		p = abs
	}
	if !strings.EqualFold(filepath.Base(p), acctConfigFileName) {
		return "", "", false
	}
	home = filepath.Dir(p)
	name = strings.ToLower(filepath.Base(home))
	if acctValidateName(name) != nil || name == "zoom-phone-cli" || !strings.EqualFold(filepath.Base(filepath.Dir(home)), acctDirName) {
		return "", "", false
	}
	return name, home, true
}

// acctIdentifyProfile reports whether cfgPath is a saved account profile.
// File identity (os.SameFile) against every <accounts>/<name>/config.toml
// catches case variants, hard links, symlinks and relative spellings; the
// path-shape check (also case-insensitive) covers profiles outside the
// operator's accounts directory and files that do not exist yet.
func acctIdentifyProfile(cfgPath string) (name, home string, ok bool) {
	if strings.TrimSpace(cfgPath) == "" {
		return "", "", false
	}
	if st, err := os.Stat(cfgPath); err == nil {
		if root, err := acctRootDir(); err == nil {
			if entries, err := os.ReadDir(root); err == nil {
				for _, e := range entries {
					candidate := filepath.Join(root, e.Name(), acctConfigFileName)
					if cst, err := os.Stat(candidate); err == nil && os.SameFile(st, cst) {
						return e.Name(), filepath.Join(root, e.Name()), true
					}
				}
			}
		}
	}
	for _, candidate := range []string{cfgPath, acctCanonicalPath(cfgPath)} {
		if name, home, ok := acctShapeHome(candidate); ok {
			return name, home, true
		}
	}
	return "", "", false
}

// acctAccountContaining reports the account directory that dir lies in: one
// of the operator's saved accounts, or any <...>/accounts/<name> that holds a
// profile (or whose data directory dir is).
func acctAccountContaining(dir string) (name, home string, ok bool) {
	if root, err := acctRootDir(); err == nil {
		if entries, err := os.ReadDir(root); err == nil {
			for _, e := range entries {
				if !e.IsDir() || acctValidateName(e.Name()) != nil {
					continue
				}
				h := filepath.Join(root, e.Name())
				if acctPathWithin(dir, h) {
					return e.Name(), h, true
				}
			}
		}
	}
	canon := acctCanonicalPath(dir)
	for p := canon; ; p = filepath.Dir(p) {
		parent := filepath.Dir(p)
		if parent == p {
			break
		}
		n := strings.ToLower(filepath.Base(p))
		if strings.EqualFold(filepath.Base(parent), acctDirName) && acctValidateName(n) == nil && n != "zoom-phone-cli" {
			st, err := os.Lstat(filepath.Join(p, acctConfigFileName))
			if (err == nil && st.Mode().IsRegular()) || strings.EqualFold(canon, filepath.Join(p, "data")) {
				return n, p, true
			}
		}
	}
	return "", "", false
}

// acctDataDirIs reports whether dataDir is exactly <home>/data.
func acctDataDirIs(dataDir, home string) bool {
	return strings.EqualFold(filepath.Base(filepath.Clean(dataDir)), "data") && acctSameDir(filepath.Dir(filepath.Clean(dataDir)), home)
}

func acctIsolationFix(name string) string {
	return fmt.Sprintf("select the account with: eval \"$(zoom-phone-cli accounts path %s --env)\"  or run: zoom-phone-cli fleet --accounts %s -- <command>", name, name)
}

// acctCheckIsolation refuses API use (configErr, exit 10) when
//   - cfgPath is an account profile but the data directory (from --home,
//     ZOOM_PHONE_HOME or ZOOM_PHONE_DATA_DIR) is not that account's data, or
//   - the data directory lies inside an account directory but cfgPath is not
//     that account's profile.
func acctCheckIsolation(cfgPath string) error {
	dataDir, dataErr := cliutil.DataDir()
	if name, home, ok := acctIdentifyProfile(cfgPath); ok {
		if dataErr != nil || !acctDataDirIs(dataDir, home) {
			return configErr(fmt.Errorf("refusing to call the API: %s is the %q account profile, but the data directory is %s, not %s (ZOOM_PHONE_HOME must be the account directory), so a token mint would move this account's secret out of its profile and the next run could act as another tenant; %s",
				cfgPath, name, dataDir, filepath.Join(home, "data"), acctIsolationFix(name)))
		}
		// The generated code rewrites the config at the path it was given, so
		// that path must be the profile inside the account directory, not a
		// symlink or hard link to it from elsewhere.
		cfgDir := filepath.Dir(filepath.Clean(cfgPath))
		if abs, err := filepath.Abs(cfgDir); err == nil {
			cfgDir = abs
		}
		if !acctSameDir(cfgDir, home) {
			return configErr(fmt.Errorf("refusing to call the API: %s reaches the %q account profile from outside its account directory (through a link), so this run would write that profile's state outside %s; use %s itself: %s",
				cfgPath, name, home, filepath.Join(home, acctConfigFileName), acctIsolationFix(name)))
		}
		return acctVerifyStorageTree(name, home)
	}
	if dataErr != nil {
		return nil
	}
	if name, home, ok := acctAccountContaining(dataDir); ok {
		return configErr(fmt.Errorf("refusing to call the API: the data directory %s belongs to the %q account (%s), but the config file %s is not that account's profile, so its credentials would be written into that account's token cache; %s, or drop --home / ZOOM_PHONE_HOME / ZOOM_PHONE_DATA_DIR to use your default profile",
			dataDir, name, home, cfgPath, acctIsolationFix(name)))
	}
	return nil
}
