// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

// Hand-written (NOT generated; handfixes.json credkit-wiring): one process per
// NAS. The credential kit lets a credential go only to the one base URL a process
// was started with, and a fleet's NAS addresses are known only at run time, so a
// command that reaches a NAS other than this process's reruns itself for that NAS.
package cli

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"synology-pp-cli/internal/cliutil"
	"synology-pp-cli/internal/config"
	"synology-pp-cli/internal/credkit"
	"synology-pp-cli/internal/dsm"
	"synology-pp-cli/internal/fleet"
)

// nasProcessEnvVar marks a process runForNAS started.
const nasProcessEnvVar = "PP_NAS_PROCESS"

// processPerNAS is set by the CLI's main alone. A test binary running Execute has
// no kit and no CLI to rerun: os.Executable is the test binary, and rerunning it
// would run the tests again.
var processPerNAS bool

// RunOneProcessPerNAS is main's switch for processPerNAS.
func RunOneProcessPerNAS() { processPerNAS = true }

// boundToNAS reports whether this process may send n's credentials: n is the
// NAS the kit was started with.
func boundToNAS(flags *rootFlags, n fleet.NAS) bool {
	if !processPerNAS {
		return true
	}
	// The environment first, as the kit reads it: a config file that does not parse
	// must not keep a registered NAS, which needs nothing from it, from its login.
	if base := os.Getenv("SYNOLOGY_BASE_URL"); base != "" {
		return dsm.SameSession(base, n.URL)
	}
	cfg, err := config.Load(flags.configPath)
	return err == nil && dsm.SameSession(cfg.BaseURL, n.URL)
}

// runForNAS runs this binary with args, bound to n: n's address as its base URL
// and only n's credentials, which the kit hands over outside the child's
// environment. It returns the child's exit code.
func runForNAS(ctx context.Context, n fleet.NAS, args []string, stdout, stderr io.Writer) (int, error) {
	// exec.CommandContext stops only the direct child, so a chain would outlive its caller.
	if os.Getenv(nasProcessEnvVar) != "" {
		return 0, fmt.Errorf("this process was started for one NAS and is not bound to %s; it starts no other", n.Name)
	}
	self, err := os.Executable()
	if err != nil {
		return 0, err
	}
	cmd := exec.CommandContext(ctx, self, args...) // #nosec G204 -- this binary, with arguments this file builds
	cmd.Env, cmd.Stdout, cmd.Stderr = nasChildEnviron(n), stdout, stderr
	err = credkit.RunCommand(cmd)
	var exit *exec.ExitError
	if errors.As(err, &exit) {
		return exit.ExitCode(), nil
	}
	return 0, err
}

// nasChildEnviron is this process's environment with the base URL replaced, without
// the MCP HTTP bearer (this server's alone) or any credential variable that is not n's.
func nasChildEnviron(n fleet.NAS) []string {
	env := []string{"SYNOLOGY_BASE_URL=" + n.URL, nasProcessEnvVar + "=1"}
	for _, entry := range os.Environ() {
		name, _, _ := strings.Cut(entry, "=")
		switch {
		case strings.EqualFold(name, "SYNOLOGY_BASE_URL"), strings.EqualFold(name, "SYNOLOGY_MCP_HTTP_TOKEN"):
		case cliutil.CredentialEnvName(name) && !n.IsCredentialEnv(name):
		default:
			env = append(env, entry)
		}
	}
	return env
}

// rerunForNAS reruns the whole command for n, whose name it adds with --nas, with
// its output passed through. The child handles --deliver, so this process no
// longer does.
func rerunForNAS(cmd *cobra.Command, flags *rootFlags, n fleet.NAS) error {
	flags.deliverBuf, flags.deliverSpec = nil, ""
	args := append(append([]string(nil), os.Args[1:]...), "--nas", n.Name)
	code, err := runForNAS(cmd.Context(), n, args, os.Stdout, os.Stderr)
	if err != nil {
		return apiErr(err)
	}
	if code != 0 {
		return silentCodeErr(code)
	}
	return nil
}

// fleetSyncChildArgs is the part of a NAS process's command line every NAS shares.
func fleetSyncChildArgs(flags *rootFlags, dbPath, window string, perNAS time.Duration, full bool) []string {
	args := []string{"fleet", "sync", "--json", "--db", dbPath, "--since", window, "--nas-timeout", perNAS.String()}
	if full {
		args = append(args, "--full")
	}
	if flags.configPath != "" {
		args = append(args, "--config", flags.configPath)
	}
	if flags.homePath != "" {
		args = append(args, "--home", flags.homePath)
	}
	if flags.timeoutExplicit {
		args = append(args, "--timeout", flags.timeout.String())
	}
	return args
}

// nasProcessGrace is how long past its own --nas-timeout a NAS process may run
// to commit what it fetched before it is stopped.
const nasProcessGrace = time.Minute

// syncNASInChild syncs n in its own process and reads back the row it reports.
// A process that reports nothing readable (refused, killed, its report withheld)
// is a failed sync of that NAS, recorded here since the child could not.
func syncNASInChild(ctx context.Context, db *sql.DB, dbPath string, n fleet.NAS, shared []string, perNAS time.Duration) fleetSyncRow {
	start := time.Now()
	ctx, cancel := context.WithTimeout(ctx, perNAS+nasProcessGrace)
	defer cancel()
	var stdout, stderr bytes.Buffer
	args := append(append([]string{}, shared...), "--nas", n.Name)
	_, err := runForNAS(ctx, n, args, &stdout, &stderr)
	var rows []fleetSyncRow
	if err == nil && json.Unmarshal(stdout.Bytes(), &rows) == nil && len(rows) == 1 {
		return rows[0]
	}
	if err == nil {
		first, _, _ := strings.Cut(strings.TrimSpace(stderr.String()), "\n")
		err = fmt.Errorf("the sync process for this NAS reported no result: %s", first)
	}
	wctx := context.WithoutCancel(ctx)
	if rerr := cliutil.WithFileLock(dbPath, func() error { return fleet.RecordAttempt(wctx, db, n, start, fleet.SystemInfo{}, err) }); rerr != nil {
		err = errors.Join(err, rerr)
	}
	return fleetSyncRow{NAS: n.Name, Client: n.Client, Error: err.Error(), DurationMS: time.Since(start).Milliseconds()}
}
