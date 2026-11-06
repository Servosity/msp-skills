// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

// aws-billing handfixes for the AWS SDK path and the helper processes the CLI
// starts:
//
//   - dry-run-sends-nothing: the AWS SDK client refuses every request while the
//     command is a --dry-run preview (awsx.DryRun reads the parsed flags).
//   - child-env-without-aws-credentials: slack-pp-cli (report --post-slack)
//     and the headless browser (report --format pdf) never need AWS
//     credentials. Every helper starts through prepareHelpers and
//     credkit.RunCommand (Codex r1 and r2 class
//     credential-handed-to-another-program): its environment is an allowlist
//     (helperEnvAllowed: the system, locale, temp, display and proxy
//     variables, XDG_*, LC_*, the Slack helper's own SLACK_* settings, the AWS
//     region; nothing else, so no AWS credential variable, alias or
//     credential-source path, and no variable of this connector); and
//     everything the runs of one operation receive (that environment, every
//     argument, stdin, a file a helper reads) is checked by the credential
//     kit as ONE unit, so a credential split between them is refused before
//     any run starts. The kit checks each child's argv and environment again
//     as it starts it.

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"aws-billing-pp-cli/internal/awsx"
	"aws-billing-pp-cli/internal/credkit"
)

func init() {
	registerNovelCommand(func(root *cobra.Command, flags *rootFlags) {
		awsx.DryRun = func() bool { return dryRunActive(flags, root) }
	})
}

// helperEnvNames and helperEnvPrefixes are the only environment variables a
// helper process inherits (names compared in upper case): what a program
// needs to run, find its own settings and reach the network.
var helperEnvNames = map[string]bool{
	"PATH": true, "HOME": true, "USER": true, "USERNAME": true, "LOGNAME": true, "SHELL": true, "LANG": true, "LANGUAGE": true,
	"TZ": true, "TERM": true, "TMPDIR": true, "TMP": true, "TEMP": true, "DISPLAY": true, "WAYLAND_DISPLAY": true,
	"XAUTHORITY": true, "DBUS_SESSION_BUS_ADDRESS": true, "SSL_CERT_FILE": true, "SSL_CERT_DIR": true,
	"HTTPS_PROXY": true, "HTTP_PROXY": true, "NO_PROXY": true, "ALL_PROXY": true,
	"AWS_REGION": true, "AWS_DEFAULT_REGION": true,
	"SYSTEMROOT": true, "WINDIR": true, "COMSPEC": true, "PATHEXT": true, "USERPROFILE": true, "APPDATA": true,
	"LOCALAPPDATA": true, "PROGRAMDATA": true, "PROGRAMFILES": true, "PROGRAMFILES(X86)": true, "PROGRAMW6432": true,
	"COMMONPROGRAMFILES": true, "HOMEDRIVE": true, "HOMEPATH": true, "SYSTEMDRIVE": true, "NUMBER_OF_PROCESSORS": true,
	"PROCESSOR_ARCHITECTURE": true, "OS": true, "PUBLIC": true, "ALLUSERSPROFILE": true,
}

var helperEnvPrefixes = []string{"XDG_", "LC_", "SLACK_"}

// helperEnvAllowed reports a variable a helper process may inherit.
func helperEnvAllowed(name string) bool {
	u := strings.ToUpper(name)
	if helperEnvNames[u] {
		return true
	}
	for _, p := range helperEnvPrefixes {
		if strings.HasPrefix(u, p) {
			return true
		}
	}
	return false
}

// awsCredentialVar names an environment variable a helper process never
// inherits: everything helperEnvAllowed does not name (every AWS_* variable
// but the region among them: the SDK's credential variables and their
// aliases, the paths and addresses of its credential sources, its profile
// selection; every AWS_BILLING_* variable).
func awsCredentialVar(name string) bool { return !helperEnvAllowed(name) }

// helperProcessEnv is the environment a helper process (not this CLI) starts
// with: this process's allowlisted variables. What it keeps is checked with
// the helper's other inputs (prepareHelpers) and again by the kit at start.
func helperProcessEnv() []string {
	base := os.Environ()
	out := make([]string, 0, len(base))
	for _, kv := range base {
		name, _, ok := strings.Cut(kv, "=")
		if !ok || awsCredentialVar(name) {
			continue
		}
		out = append(out, kv)
	}
	return out
}

// helperRun is one helper process of an operation: its command, its stdin,
// and the content of any file it is given to read.
type helperRun struct {
	cmd   *exec.Cmd
	stdin []byte
	reads [][]byte
}

// prepareHelpers is the one way helper processes start: every run gets
// helperProcessEnv and its stdin, and none starts when everything the runs of
// this operation receive (the environment, every argument of every run, every
// stdin and file read), put together as one unit, is withheld by the
// credential kit (withheldByKit). The receiver of several runs (Slack gets
// the message and the upload) can put them together, so they are one check.
// Each run then starts through credkit.RunCommand.
func prepareHelpers(runs ...helperRun) error {
	env := helperProcessEnv()
	unit := append([]string(nil), env...)
	var names []string
	for _, r := range runs {
		unit = append(unit, r.cmd.Args...)
		if r.stdin != nil {
			unit = append(unit, string(r.stdin))
		}
		for _, b := range r.reads {
			unit = append(unit, string(b))
		}
		names = append(names, filepath.Base(r.cmd.Path))
	}
	if withheldByKit([]byte(strings.Join(unit, "\n"))) {
		return fmt.Errorf("%s refused: its environment, arguments or input would repeat a credential this program holds", strings.Join(names, ", "))
	}
	for _, r := range runs {
		r.cmd.Env = env
		if r.stdin != nil {
			r.cmd.Stdin = bytes.NewReader(r.stdin)
		}
	}
	return nil
}

// withheldByKit reports whether the credential kit withholds b as one unit
// sent to a child process. The kit's own matcher decides (a kit Writer over
// a buffer, which receives b or the kit's notice); nothing is written
// anywhere else.
func withheldByKit(b []byte) bool {
	var buf bytes.Buffer
	_, err := credkit.NewWriter(&buf, credkit.SinkChild).Write(b)
	return err != nil || !bytes.Equal(buf.Bytes(), b)
}

// prepareHelper prepares one helper run (prepareHelpers).
func prepareHelper(cmd *exec.Cmd, stdin []byte) error {
	return prepareHelpers(helperRun{cmd: cmd, stdin: stdin})
}
