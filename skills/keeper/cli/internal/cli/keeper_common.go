// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

// keeper handfix commander-allowlist (CLI glue): every typed read builds its
// keeper.Command first (arguments are validated before any request or
// preview), returns the shared Service Mode session to the MSP (or switches
// into one managed company and back), sends the one command through the
// generated client (api-key header, origin pin, credential kit) and
// prints only the command's allowlisted fields.

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/url"
	"os"
	"os/user"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"keeper-pp-cli/internal/client"
	"keeper-pp-cli/internal/keeper"

	"github.com/spf13/cobra"
)

func init() {
	// posture report saves Keeper data to the local store, like sync:
	// KEEPER_NO_LOCAL_DATA=1 refuses it (root.go applyLocalDataEnv).
	localDataWriterCommands["posture report"] = true
}

// keeperPoster sends through the generated client's POST, which never
// retries an ambiguous failure: a context switch must not be replayed, and
// Service Mode answers "Already MSP" with HTTP 500 on every MSP-level read.
type keeperPoster struct {
	c    *client.Client
	lock string
}

func (p keeperPoster) Post(ctx context.Context, path string, body any) (json.RawMessage, int, error) {
	return p.c.Post(ctx, path, body)
}

// SessionLockFile names this instance's session lock (keeper.LockedPoster).
func (p keeperPoster) SessionLockFile() string { return p.lock }

// keeperCommandTimeout is Service Mode's own request_timeout (300 s,
// keepercommander/service/core/request_queue.py): a large security audit can
// take minutes. An explicit --timeout wins.
const keeperCommandTimeout = 5 * time.Minute

func newKeeperPoster(flags *rootFlags) (keeper.Poster, error) {
	saved := flags.timeout
	if !flags.timeoutExplicit && flags.timeout < keeperCommandTimeout {
		flags.timeout = keeperCommandTimeout
	}
	c, err := flags.newClient()
	flags.timeout = saved
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(c.Config.AuthHeader()) == "" {
		return nil, authErr(fmt.Errorf("no Service Mode API key: set KEEPER_API_KEY (the key service-create printed) or run 'keeper-cli auth set-token'"))
	}
	lock, err := sessionLockFile(c.BaseURL)
	if err != nil {
		return nil, err
	}
	return keeperPoster{c: c, lock: lock}, nil
}

// sessionLockFile is the file every process of this CLI for this user locks for
// one context transaction against one Service Mode instance (keeper handfix
// mc-context-serialized). It is keyed to the normalized origin in a directory that
// depends only on the OS user, never on --home, HOME, XDG_CACHE_HOME, TMPDIR or the
// per-kind variables, which differ between two runs of one user. Every loopback
// spelling is one instance. Fail closed: no lock, no read.
func sessionLockFile(baseURL string) (string, error) {
	u, err := url.Parse(baseURL)
	if err != nil {
		return "", fmt.Errorf("reading the Service Mode base URL for the session lock: %w", err)
	}
	origin := normalizedOrigin(u)
	if origin == "" {
		return "", fmt.Errorf("reading the Service Mode base URL for the session lock: no host")
	}
	scheme, hostPort, _ := strings.Cut(origin, "://")
	host, port, err := net.SplitHostPort(hostPort)
	if err != nil {
		return "", fmt.Errorf("reading the Service Mode base URL for the session lock: %w", err)
	}
	host = strings.TrimSuffix(host, ".")
	if ip := net.ParseIP(host); host == "localhost" || (ip != nil && ip.IsLoopback()) {
		host = "loopback"
	}
	key := scheme + "://" + net.JoinHostPort(host, port)
	dir, err := sessionLockDir()
	if err != nil {
		return "", fmt.Errorf("preparing the Service Mode session lock directory: %w", err)
	}
	sum := sha256.Sum256([]byte(key))
	return filepath.Join(dir, "service-mode-"+hex.EncodeToString(sum[:8])), nil
}

// sessionLockDir is the OS user's lock directory. On Windows the profile comes from
// the process token. Elsewhere the uid names it under /tmp; a directory that is not
// a private directory (a symlink, or another user's) is refused.
func sessionLockDir() (string, error) {
	if runtime.GOOS == "windows" {
		u, err := user.Current()
		if err != nil {
			return "", err
		}
		dir := filepath.Join(u.HomeDir, "AppData", "Local", "keeper-cli")
		return dir, os.MkdirAll(dir, 0o700)
	}
	dir := fmt.Sprintf("/tmp/keeper-cli-%d", os.Getuid())
	if err := os.Mkdir(dir, 0o700); err != nil && !errors.Is(err, os.ErrExist) {
		return "", err
	}
	info, err := os.Lstat(dir)
	if err != nil {
		return "", err
	}
	if !info.IsDir() || info.Mode().Perm() != 0o700 {
		return "", fmt.Errorf("%s is not a private directory", dir)
	}
	return dir, nil
}

// validateMC checks --mc before anything else runs.
func validateMC(mc string) (string, error) {
	if mc == "" {
		return "", nil
	}
	if mc == selfCompanyID {
		return mc, nil
	}
	v, err := keeper.CompanyID(mc)
	if err != nil {
		return "", usageErr(err)
	}
	return v, nil
}

// contextNote is the dry-run and help wording for the session switch.
func contextNote(mc string) string {
	if mc == "" {
		return "after returning the Service Mode session to the MSP"
	}
	return "inside managed company " + mc + " (switch-to-mc " + mc + ", then switch-to-msp)"
}

// readKeeperRows runs one allowlisted report in the MSP context or in
// managed company mc and returns its projected rows.
func readKeeperRows(cmd *cobra.Command, p keeper.Poster, mc string, kc keeper.Command) ([]keeper.Row, error) {
	var rows []keeper.Row
	warn := func(msg string) { fmt.Fprintln(cmd.ErrOrStderr(), "note: "+msg) }
	err := keeper.WithCompany(cmd.Context(), p, mc, warn, func() error {
		var rerr error
		rows, rerr = keeper.Run(cmd.Context(), p, kc)
		return rerr
	})
	if err != nil {
		return nil, apiErr(err)
	}
	return rows, nil
}

// runKeeperReport is the whole body of a typed read command.
func runKeeperReport(cmd *cobra.Command, flags *rootFlags, mc string, build func() (keeper.Command, error)) error {
	if err := validateDataSourceStrategy(flags, "live"); err != nil {
		return err
	}
	mc, err := validateMC(mc)
	if err != nil {
		return err
	}
	if mc == selfCompanyID {
		mc = "" // the account's own enterprise
	}
	kc, err := build()
	if err != nil {
		return usageErr(err)
	}
	if dryRunOK(flags) {
		return writeDryRun(cmd.OutOrStdout(), flags, "send the Commander command '"+kc.String()+"' "+contextNote(mc))
	}
	p, err := newKeeperPoster(flags)
	if err != nil {
		return err
	}
	rows, err := readKeeperRows(cmd, p, mc, kc)
	if err != nil {
		return err
	}
	return printKeeperRows(cmd, flags, rows)
}

// printKeeperRows prints rows through the generated output pipeline
// (--json, --agent, --select, --csv, --plain, tables).
func printKeeperRows(cmd *cobra.Command, flags *rootFlags, rows []keeper.Row) error {
	if rows == nil {
		rows = []keeper.Row{}
	}
	raw, err := json.Marshal(rows)
	if err != nil {
		return err
	}
	return printOutputWithFlagsMeta(cmd.OutOrStdout(), raw, flags, map[string]any{"source": "live"})
}

// fixed adapts a constructor without arguments.
func fixed(c keeper.Command) func() (keeper.Command, error) {
	return func() (keeper.Command, error) { return c, nil }
}
