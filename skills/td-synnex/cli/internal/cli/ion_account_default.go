// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-written: let account-scoped endpoint commands omit the <accountId>
// positional when TD_SYNNEX_ACCOUNT_ID (or the config file) supplies it.
// A reseller has one account ID, and every account path carries it.

package cli

import (
	"net/url"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	"td-synnex-pp-cli/internal/config"
)

const accountPathPrefix = "/accounts/{accountId}"

func init() {
	// Key the local store on the reseller account, not on a hash of the access
	// token: auth mint rotates the token every two hours, which would otherwise
	// start a new empty database each time. Token-named stores are adopted
	// through the legacy scope path.
	config.StoreScopeIdentity = func(c *config.Config) string {
		a := strings.TrimSpace(c.TemplateVars["accountId"])
		if a == "" {
			return ""
		}
		// Production keeps the plain account key; any other host (a test server or
		// proxy) gets its own store so its data never mixes with production's.
		if u, err := url.Parse(c.BaseURL); err == nil && u.Host != "" && !strings.EqualFold(u.Hostname(), "ion.tdsynnex.com") {
			// Another prefix and quoted parts: no account ID can make a production
			// key equal a test host's, or two test keys equal (adversarial round 30).
			return "ion-account@" + strconv.Quote(strings.ToLower(u.Host)) + ":" + strconv.Quote(a)
		}
		return "ion-account:" + a
	}
	registerNovelCommand(func(root *cobra.Command, flags *rootFlags) {
		walkCommands(root, func(c *cobra.Command) { wrapAccountDefault(c, flags) })
	})
}

// wrapAccountDefault lets c omit its leading <accountId> positional. Novel
// commands registered after this file's hook (hooks run in file-name order)
// call it themselves; the annotation keeps a second call a no-op.
func wrapAccountDefault(c *cobra.Command, flags *rootFlags) {
	optedIn := strings.HasPrefix(c.Annotations["pp:path"], accountPathPrefix) || c.Annotations["td:account-positional"] == "true"
	if !optedIn || c.RunE == nil || c.Annotations["td:account-wrapped"] == "true" || !strings.HasPrefix(strings.TrimPrefix(c.Use, c.Name()+" "), "<accountId>") {
		return
	}
	positionals := strings.Count(c.Use, "<")
	orig := c.RunE
	c.RunE = func(cmd *cobra.Command, args []string) error {
		return orig(cmd, withDefaultAccountID(flags, args, positionals))
	}
	if c.Annotations == nil {
		c.Annotations = map[string]string{}
	}
	c.Annotations["td:account-wrapped"] = "true"
	if strings.Contains(c.Long, "TD_SYNNEX_ACCOUNT_ID") {
		return
	}
	base := c.Long
	if base == "" {
		base = c.Short
	}
	c.Long = base + "\n\nOmit <accountId> when TD_SYNNEX_ACCOUNT_ID is set."
}

// withDefaultAccountID prepends the configured account ID when the caller
// passed every positional except the leading <accountId>.
func withDefaultAccountID(flags *rootFlags, args []string, positionals int) []string {
	if positionals < 1 || len(args) != positionals-1 {
		return args
	}
	cfg, err := config.Load(flags.configPath)
	if err != nil {
		return args // ponytail: the wrapped command reports config errors itself
	}
	acct := strings.TrimSpace(cfg.TemplateVars["accountId"])
	if acct == "" {
		return args
	}
	return append([]string{acct}, args...)
}

func walkCommands(c *cobra.Command, fn func(*cobra.Command)) {
	fn(c)
	for _, child := range c.Commands() {
		walkCommands(child, fn)
	}
}
