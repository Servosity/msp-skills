// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// pp:data-source local
// Saves one client-account profile (a Zoom Server-to-Server OAuth app) under
// <config dir>/accounts/<name>/config.toml. Layout and isolation rules live in
// zoom_accounts.go and zoom_accounts_guard.go.

package cli

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/pelletier/go-toml/v2"
	"github.com/spf13/cobra"

	"zoom-phone-pp-cli/internal/cliutil"
	"zoom-phone-pp-cli/internal/config"
)

// acctMaxSecretBytes bounds what stdin may carry. Zoom client secrets are 32
// characters; anything near this limit is a wrong pipe.
const acctMaxSecretBytes = 4096

// acctConfigFile is what accounts connect persists. The keys match the generated
// config.Config TOML tags, so config.Load on this file yields ClientID,
// ClientSecret and ZoomPhoneAccountId directly.
type acctConfigFile struct {
	ClientID     string `toml:"client_id"`
	ClientSecret string `toml:"client_secret"`
	AccountID    string `toml:"phone_account_id"`
}

func newNovelAccountsConnectCmd(flags *rootFlags) *cobra.Command {
	var flagAccountId string
	var flagClientId string
	var flagClientSecretStdin bool
	var flagClientSecretEnv bool
	var flagForce bool

	cmd := &cobra.Command{
		Use:     "connect [name]",
		Aliases: []string{"add"},
		Short:   "Save one profile per client Zoom account so you can switch tenants without re-entering credentials.",
		Long: `Save one client Zoom account's Server-to-Server OAuth app (account ID, client ID and client secret) as a named profile. Do NOT use it to run a command across accounts; use 'fleet' instead.

Each client Zoom account needs its own Server-to-Server OAuth app. The profile is
private (directory 0700, file 0600), and every profile keeps its own cached
token and its own local store, so tenants never share data. A symlinked
accounts or account directory is refused.

The client secret comes from stdin with --client-secret-stdin, or from the
ZOOM_PHONE_CLIENT_SECRET environment variable with --client-secret-env. Without
either flag, stdin is used only when it is a non-empty file or a pipe that
delivers the secret within a few seconds; a terminal or idle stdin is an error.
The secret is never accepted as a flag value.`,
		Example: strings.Trim(`
  printf '%s' "$ACME_SECRET" | zoom-phone-cli accounts connect acme --account-id abc123XYZ --client-id cid123 --client-secret-stdin
  ZOOM_PHONE_CLIENT_SECRET=... zoom-phone-cli accounts connect globex --account-id def456UVW --client-id cid456 --client-secret-env
  zoom-phone-cli accounts connect acme --account-id abc123XYZ --client-id cid789 --force < secret.txt`, "\n"),
		// mcp:hidden: writing a credential profile must not be an agent tool.
		Annotations: map[string]string{
			"mcp:hidden":          "true",
			"pp:data-source":      "local",
			"pp:happy-args":       "name=acme;--account-id=abc123XYZ;--client-id=cid123",
			"pp:typed-exit-codes": "0,2",
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 && cmd.Flags().NFlag() == 0 {
				return cmd.Help()
			}
			if dryRunOK(flags) {
				return writeDryRun(cmd.OutOrStdout(), flags, "accounts connect")
			}
			if len(args) != 1 {
				_ = cmd.Usage()
				return usageErr(errors.New("exactly one account name is required, e.g. zoom-phone-cli accounts connect acme ..."))
			}
			name := args[0]
			if err := acctValidateName(name); err != nil {
				return usageErr(err)
			}
			accountID := strings.TrimSpace(flagAccountId)
			clientID := strings.TrimSpace(flagClientId)
			if accountID == "" || clientID == "" {
				_ = cmd.Usage()
				return usageErr(errors.New("--account-id and --client-id are required (both are on the Server-to-Server OAuth app's App Credentials page)"))
			}
			if strings.ContainsAny(accountID+clientID, " \t\r\n") {
				return usageErr(errors.New("--account-id and --client-id must not contain whitespace"))
			}
			secret, err := acctReadSecret(cmd.InOrStdin(), flagClientSecretStdin, flagClientSecretEnv)
			if err != nil {
				return usageErr(err)
			}
			cliutil.Hold("stdin", secret) // Hand-fix (handfixes.json credkit-wiring): the secret is held the moment it is read

			home, err := acctHomeDir(name)
			if err != nil {
				return usageErr(err)
			}
			// --force must not write through a tampered tree into another account.
			if err := acctVerifyStorageTree(name, home); err != nil {
				return err
			}
			// Every writer of a profile takes the same lock (config.ZoomProfileLockPath:
			// <accounts>/.locks/<name>.lock, outside the removable account tree), for
			// a first-time profile too, before preparing the directory.
			path := filepath.Join(home, acctConfigFileName)
			release, lockErr := config.ZoomAcquireProfileLock(path)
			if lockErr != nil {
				return configErr(fmt.Errorf("locking account %q: %w", name, lockErr))
			}
			defer release()
			if err := acctPrepareHome(home); err != nil {
				return err
			}
			// Under the lock, re-check: another connect or a token save may have
			// created or changed the profile, or tampered with its tree, meanwhile.
			if err := acctVerifyStorageTree(name, home); err != nil {
				return err
			}
			if _, err := os.Lstat(path); err == nil {
				if !flagForce {
					return usageErr(fmt.Errorf("account %q already exists at %s; pass --force to overwrite it", name, path))
				}
			} else if !errors.Is(err, fs.ErrNotExist) {
				return err
			}
			if err := acctWriteConfig(home, path, acctConfigFile{ClientID: clientID, ClientSecret: secret, AccountID: accountID}); err != nil {
				return err
			}
			// A token cached for any previous credentials must not outlive them
			// (still under the lock).
			if credsPath, err := cliutil.CredentialsFilePathForConfig(path); err == nil {
				if err := os.Remove(credsPath); err != nil && !errors.Is(err, fs.ErrNotExist) {
					return fmt.Errorf("removing stale cached token %s: %w", credsPath, err)
				}
			}

			result := map[string]any{
				"name":          name,
				"path":          path,
				"account_id":    accountID,
				"has_client_id": clientID != "", // credkit-wiring: half of the Basic credential, held, so never echoed
				"has_secret":    secret != "",
			}
			if !wantsHumanTable(cmd.OutOrStdout(), flags) {
				return printJSONFilteredKeep(cmd.OutOrStdout(), result, flags, "name", "path", "account_id", "has_client_id", "has_secret")
			}
			fmt.Fprintf(cmd.OutOrStdout(), "saved account %s (account %s) to %s\n", name, accountID, path)
			fmt.Fprintf(cmd.OutOrStdout(), "run across accounts: zoom-phone-cli fleet -- doctor\n")
			return nil
		},
	}
	cmd.Flags().StringVar(&flagAccountId, "account-id", "", "Zoom account ID of the client's Server-to-Server OAuth app")
	cmd.Flags().StringVar(&flagClientId, "client-id", "", "Client ID of the client's Server-to-Server OAuth app")
	cmd.Flags().BoolVar(&flagClientSecretStdin, "client-secret-stdin", false, "Read the client secret from stdin, waiting until stdin closes")
	cmd.Flags().BoolVar(&flagClientSecretEnv, "client-secret-env", false, "Read the client secret from the ZOOM_PHONE_CLIENT_SECRET environment variable instead of stdin")
	cmd.Flags().BoolVar(&flagForce, "force", false, "Overwrite an existing profile with the same name")
	return cmd
}

// acctImplicitStdinWait bounds how long the default (flagless) stdin read waits
// on a pipe for the secret's line or EOF, so an inherited stdin that is open
// but never written (an agent harness, a backgrounded shell) cannot hang the
// command. --client-secret-stdin waits indefinitely.
var acctImplicitStdinWait = 3 * time.Second

var errAcctStdinWait = errors.New("stdin wait expired")

// #nosec G101 -- usage hint that names the secret flags; it holds no secret.
const acctSecretFlagHint = "pipe it with --client-secret-stdin (printf '%s' \"$SECRET\" | zoom-phone-cli accounts connect <name> ... --client-secret-stdin) or pass --client-secret-env; it is never accepted as a flag value"

// acctReadSecret returns the client secret from stdin or, only with
// --client-secret-env, from ZOOM_PHONE_CLIENT_SECRET. Without either flag,
// stdin is read only when it is a non-empty file or a pipe that delivers a
// line (or EOF) within acctImplicitStdinWait; a terminal, /dev/null or an idle
// pipe is an error naming the flags. It never accepts a value from argv.
func acctReadSecret(stdin io.Reader, fromStdin, fromEnv bool) (string, error) {
	if fromStdin && fromEnv {
		return "", errors.New("use only one of --client-secret-stdin and --client-secret-env")
	}
	if fromEnv {
		secret := strings.TrimSpace(cliutil.EnvOverride("ZOOM_PHONE_CLIENT_SECRET"))
		if secret == "" {
			return "", errors.New("--client-secret-env: ZOOM_PHONE_CLIENT_SECRET is not set")
		}
		return secret, nil
	}
	envHint := ""
	if strings.TrimSpace(cliutil.EnvOverride("ZOOM_PHONE_CLIENT_SECRET")) != "" {
		envHint = "; ZOOM_PHONE_CLIENT_SECRET is set but is only used with --client-secret-env"
	}
	if stdin == nil {
		return "", errors.New("client secret required on stdin, but stdin is not available; " + acctSecretFlagHint + envHint)
	}
	var data []byte
	var err error
	f, isFile := stdin.(*os.File)
	switch {
	case !isFile || fromStdin:
		data, err = io.ReadAll(io.LimitReader(stdin, acctMaxSecretBytes+1))
	default:
		st, statErr := f.Stat()
		if statErr != nil {
			return "", fmt.Errorf("client secret required: cannot inspect stdin (%v); %s%s", statErr, acctSecretFlagHint, envHint)
		}
		switch mode := st.Mode(); {
		case mode.IsRegular():
			if st.Size() == 0 {
				return "", errors.New("client secret required: stdin is an empty file; " + acctSecretFlagHint + envHint)
			}
			data, err = io.ReadAll(io.LimitReader(f, acctMaxSecretBytes+1))
		case mode&(os.ModeNamedPipe|os.ModeSocket) != 0:
			data, err = acctReadLineWithin(f, acctImplicitStdinWait)
			if errors.Is(err, errAcctStdinWait) {
				return "", fmt.Errorf("client secret required: nothing arrived on stdin within %s; %s%s", acctImplicitStdinWait, acctSecretFlagHint, envHint)
			}
		default: // a terminal, /dev/null or another device: no secret to read
			return "", errors.New("client secret required: " + acctSecretFlagHint + envHint)
		}
	}
	if err != nil {
		return "", fmt.Errorf("reading the client secret from stdin: %w", err)
	}
	if len(data) > acctMaxSecretBytes {
		return "", fmt.Errorf("stdin exceeds %d bytes; pipe only the client secret", acctMaxSecretBytes)
	}
	secret := strings.TrimSpace(string(data))
	if secret == "" {
		return "", errors.New("client secret required: stdin was empty; " + acctSecretFlagHint + envHint)
	}
	if strings.ContainsAny(secret, "\r\n") {
		return "", errors.New("expected one line on stdin containing only the client secret")
	}
	return secret, nil
}

// acctReadLineWithin reads up to the first newline (or EOF) from r, giving up
// with errAcctStdinWait after wait. On timeout the reader goroutine stays
// blocked until the process exits, which follows immediately.
func acctReadLineWithin(r io.Reader, wait time.Duration) ([]byte, error) {
	type result struct {
		data []byte
		err  error
	}
	ch := make(chan result, 1)
	go func() {
		br := bufio.NewReader(io.LimitReader(r, acctMaxSecretBytes+1))
		line, err := br.ReadBytes('\n')
		if errors.Is(err, io.EOF) {
			err = nil
		}
		ch <- result{data: line, err: err}
	}()
	select {
	case res := <-ch:
		return res.data, res.err
	case <-time.After(wait):
		return nil, errAcctStdinWait
	}
}

// acctPrepareHome creates the accounts and account directories (0700),
// refusing a symlink or non-directory at either, before and after creation.
func acctPrepareHome(home string) error {
	if err := acctRefuseSymlinkDir(filepath.Dir(home)); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(home), 0o700); err != nil {
		return fmt.Errorf("creating accounts directory: %w", err)
	}
	if err := acctRefuseSymlinkDir(home); err != nil {
		return err
	}
	if err := os.MkdirAll(home, 0o700); err != nil {
		return fmt.Errorf("creating account directory: %w", err)
	}
	if err := acctRefuseSymlinkDir(home); err != nil {
		return err
	}
	if err := acctRefuseSymlinkDir(filepath.Dir(home)); err != nil {
		return err
	}
	if err := os.Chmod(home, 0o700); err != nil { // #nosec G302 -- a directory: owner-only rwx (0700) is the tightest usable mode
		return fmt.Errorf("securing account directory: %w", err)
	}
	return nil
}

// acctWriteConfig creates the account directory (0700) and atomically writes
// the profile (0600), then proves the file is private before reporting success.
// Callers have already refused symlinked accounts/account directories; the
// checks repeat here because MkdirAll would follow a link created in between.
func acctWriteConfig(home, path string, cfg acctConfigFile) error {
	if err := acctPrepareHome(home); err != nil {
		return err
	}
	data, err := toml.Marshal(cfg) // #nosec G117 -- the client secret is intentionally persisted to a 0600 private file.
	if err != nil {
		return fmt.Errorf("encoding account profile: %w", err)
	}
	if err := cliutil.AtomicWriteCredentialFile(path, data, 0o600, 0o700); err != nil {
		return fmt.Errorf("writing account profile: %w", err)
	}
	real, err := filepath.EvalSymlinks(path)
	if err != nil {
		return fmt.Errorf("resolving account profile: %w", err)
	}
	if err := cliutil.VerifyCredsPerms(real); err != nil {
		return fmt.Errorf("account profile %s is not private: %w", path, err)
	}
	return nil
}
