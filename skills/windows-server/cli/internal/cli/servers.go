// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
//
// Hand-written (NOT generated): `servers list|login|logout`. Recorded in
// handfixes.json (server-profiles).

package cli

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"maps"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"windows-server-pp-cli/internal/cliutil"
	"windows-server-pp-cli/internal/config"
	"windows-server-pp-cli/internal/credkit"
	"windows-server-pp-cli/internal/winsrv"
)

func newServersCmd(flags *rootFlags) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "servers",
		Short: "List configured servers and store a credential per server",
		Long: `Servers come from the config file's [[servers]] list:

  [[servers]]
  name = "fs01"                 # what --server takes
  host = "fs01.corp.example"    # DNS name or IP, nothing else
  port = 5986                   # default 5986 (HTTPS)
  auth = "ntlm"                 # ntlm (default) or basic
  username = "CORP\\svc-monitor"
  cert_sha256 = "4B:EF:..."     # pin the WinRM certificate, or set ca_file
  default = true

The shared account (auth set-credentials, or WINDOWS_SERVER_USERNAME and
WINDOWS_SERVER_PASSWORD) is used for every server without its own entry.`,
		Annotations: map[string]string{"pp:parent-group": "true", "pp:typed-exit-codes": "0,2"},
		RunE:        parentNoSubcommandRunE(flags),
	}
	cmd.AddCommand(newServersListCmd(flags), newServersLoginCmd(flags), newServersLogoutCmd(flags))
	return cmd
}

func newServersListCmd(flags *rootFlags) *cobra.Command {
	return &cobra.Command{
		Use:     "list",
		Short:   "List configured servers and where each one's credential comes from (never the value)",
		Example: "  windows-server-cli servers list --agent",
		Args:    cobra.NoArgs,
		// pp:data-source local
		Annotations: map[string]string{"mcp:read-only": "true", "pp:data-source": "local"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return writeDryRun(cmd.OutOrStdout(), flags, "servers list")
			}
			cfg, err := config.Load(flags.configPath)
			if err != nil {
				return configErr(err)
			}
			profiles, credErrs, err := serversFromConfig(cfg, "", true)
			if errors.Is(err, winsrv.ErrNoServers) {
				// An empty list is an answer, not a failure: say how to add one.
				fmt.Fprintln(cmd.ErrOrStderr(), err)
				return printWinsrv(cmd, flags, []map[string]any{}, "name", "host", "credential")
			}
			if err != nil {
				return err
			}
			rows := make([]map[string]any, 0, len(profiles))
			for _, p := range profiles {
				cred := p.CredentialFrom
				if err := credErrs[p.Name]; err != nil {
					cred = "missing: " + err.Error()
				}
				trust := "system roots"
				switch {
				case p.CertSHA256 != "":
					trust = "pinned certificate"
				case p.CAFile != "":
					trust = "ca_file"
				}
				rows = append(rows, map[string]any{
					"name": p.Name, "host": p.Host, "port": p.Port, "transport": p.Transport, "auth": p.Auth,
					"username": p.Username, "tls_trust": trust, "default": p.Default, "credential": cred,
				})
			}
			return printWinsrv(cmd, flags, rows, "name", "host", "credential")
		},
	}
}

func newServersLoginCmd(flags *rootFlags) *cobra.Command {
	var username string
	cmd := &cobra.Command{
		Use:   "login <server>",
		Short: "Store a credential for one server (password read from stdin)",
		Long: `Stores a username and password for one configured server in the 0600
credential store next to the shared credentials file. The password is read
from standard input, never from an argument, so it does not land in shell
history or the process list:

  security find-generic-password -s fs01-winrm -w | windows-server-cli servers login fs01 --username 'CORP\svc-monitor'`,
		Example: "  windows-server-cli servers login fs01 --username 'CORP\\svc-monitor' < password.txt",
		Args:    cobra.ExactArgs(1),
		// pp:data-source local
		Annotations: map[string]string{"mcp:hidden": "true", "pp:data-source": "local"},
		RunE: func(cmd *cobra.Command, args []string) error {
			name := args[0]
			if !winsrv.ProfileNameRE.MatchString(name) {
				return usageErr(fmt.Errorf("%q is not a server name", name))
			}
			if dryRunOK(flags) {
				return writeDryRun(cmd.OutOrStdout(), flags, "servers login "+name)
			}
			if f, ok := cmd.InOrStdin().(*os.File); ok && isTerminal(f) {
				return usageErr(errors.New("pipe the password on stdin (typing it here would echo it): <password-source> | windows-server-cli servers login " + name + " --username <user>"))
			}
			line, err := bufio.NewReader(io.LimitReader(cmd.InOrStdin(), 4096)).ReadString('\n')
			if err != nil && !errors.Is(err, io.EOF) {
				return err
			}
			password := strings.TrimRight(line, "\r\n")
			credkit.Hold("stdin", password) // Hand-fix (handfixes.json credkit-wiring): acquired after Start, from stdin
			if winsrv.PasswordTooShort(password) {
				return usageErr(fmt.Errorf("the password on stdin is empty or shorter than %d characters", winsrv.MinPasswordLen))
			}
			cfg, err := config.Load(flags.configPath)
			if err != nil {
				return configErr(err)
			}
			profiles, err := winsrv.LoadProfiles(cfg.Path)
			if err != nil {
				return configErr(err)
			}
			sel, err := winsrv.Select(profiles, name, false)
			if err != nil {
				return usageErr(err)
			}
			if strings.TrimSpace(username) == "" {
				username = sel[0].Username
			}
			if strings.TrimSpace(username) == "" {
				return usageErr(fmt.Errorf("no username: pass --username or set username on server %s in the config", name))
			}
			path, err := serverCredentialsPath()
			if err != nil {
				return configErr(err)
			}
			creds, err := winsrv.LoadServerCredentials(path)
			if err != nil {
				return configErr(err)
			}
			creds[winsrv.CredentialKey(sel[0])] = winsrv.ServerCredential{Username: username, Password: password}
			if err := winsrv.SaveServerCredentials(path, creds); err != nil {
				return configErr(err)
			}
			return printWinsrv(cmd, flags, map[string]any{"saved": true, "server": name, "username": username, "credentials_path": path})
		},
	}
	cmd.Flags().StringVar(&username, "username", "", "Account to sign in with (DOMAIN\\user, user@domain, or a local user); default: the server's username in the config")
	return cmd
}

func newServersLogoutCmd(flags *rootFlags) *cobra.Command {
	return &cobra.Command{
		Use:     "logout <server>",
		Short:   "Remove one server's stored credential",
		Example: "  windows-server-cli servers logout fs01",
		Args:    cobra.ExactArgs(1),
		// pp:data-source local
		Annotations: map[string]string{"mcp:hidden": "true", "pp:data-source": "local"},
		RunE: func(cmd *cobra.Command, args []string) error {
			name := args[0]
			if !winsrv.ProfileNameRE.MatchString(name) {
				return usageErr(fmt.Errorf("%q is not a server name", name))
			}
			if dryRunOK(flags) {
				return writeDryRun(cmd.OutOrStdout(), flags, "servers logout "+name)
			}
			cfg, err := config.Load(flags.configPath)
			if err != nil {
				return configErr(err)
			}
			configFile := winsrv.CanonicalPath(cfg.Path)
			path, err := serverCredentialsPath()
			if err != nil {
				return configErr(err)
			}
			creds, err := winsrv.LoadServerCredentials(path)
			if err != nil {
				return configErr(err)
			}
			// Every entry this config saved for the name, at any host; another
			// config's server of the same name keeps its credential.
			prefix := winsrv.CredentialPrefix(configFile, name)
			n := len(creds)
			maps.DeleteFunc(creds, func(k string, _ winsrv.ServerCredential) bool { return strings.HasPrefix(k, prefix) })
			had := len(creds) < n
			if had {
				if err := winsrv.SaveServerCredentials(path, creds); err != nil {
					return configErr(err)
				}
			}
			return printWinsrv(cmd, flags, map[string]any{"removed": had, "server": name})
		},
	}
}

func serverCredentialsPath() (string, error) {
	p, err := cliutil.CredentialsFilePath()
	if err != nil {
		return "", err
	}
	return winsrv.ServerCredentialsPath(p), nil
}
