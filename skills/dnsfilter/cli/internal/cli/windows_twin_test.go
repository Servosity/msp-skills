// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

// Every POSIX `export DNSFILTER_...` hint the CLI prints names the PowerShell
// form on the same or the next line: most MSPs run Windows.
func TestAuthHintsCarryPowerShellTwin(t *testing.T) {
	for _, dir := range []string{".", "../client", "../config", "../cliutil"} {
		files, _ := filepath.Glob(filepath.Join(dir, "*.go"))
		for _, f := range files {
			if strings.HasSuffix(f, "_test.go") {
				continue
			}
			data, err := os.ReadFile(f)
			if err != nil {
				t.Fatal(err)
			}
			lines := strings.Split(string(data), "\n")
			for i, line := range lines {
				if strings.HasPrefix(strings.TrimSpace(line), "//") || !strings.Contains(line, "export DNSFILTER_") {
					continue
				}
				next := ""
				if i+1 < len(lines) {
					next = lines[i+1]
				}
				if !strings.Contains(line, "PowerShell") && !strings.Contains(next, "PowerShell") {
					t.Errorf("%s:%d: POSIX-only hint with no PowerShell twin: %s", f, i+1, strings.TrimSpace(line))
				}
			}
		}
	}
}

// Help examples that use a POSIX-only construct (`< file` redirection, `$(...)`
// substitution) carry a PowerShell line.
func TestHelpExamplesCarryPowerShellTwin(t *testing.T) {
	var walk func(c *cobra.Command)
	walk = func(c *cobra.Command) {
		if (strings.Contains(c.Example, " < ") || strings.Contains(c.Example, "$(")) && !strings.Contains(c.Example, "# PowerShell") {
			t.Errorf("%q example is POSIX-only:\n%s", c.CommandPath(), c.Example)
		}
		for _, child := range c.Commands() {
			walk(child)
		}
	}
	walk(RootCmd())
}

func TestAuthSetupPrintsBothShells(t *testing.T) {
	cmd := RootCmd()
	cmd.SetArgs([]string{"auth", "setup"})
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`export DNSFILTER_API_KEY="your-token-here"`, `$env:DNSFILTER_API_KEY = "your-token-here"`} {
		if !strings.Contains(out.String(), want) {
			t.Fatalf("auth setup lacks %q:\n%s", want, out.String())
		}
	}
}
