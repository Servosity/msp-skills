// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import "dnsfilter-pp-cli/internal/cliutil"

// humanText makes a tenant string safe for the novel commands' human output.
// Organization, policy and roaming-client names, agent-reported hostnames and
// versions, query-log domains and end-user unblock requests all come from the
// account, and a DNS query name can carry any byte, so ESC/OSC 52 or a C1
// control in one could rewrite the operator's terminal or clipboard. Controls
// are dropped and tabs and newlines become spaces, so a value cannot forge an
// extra output line. (%q arguments are already escaped by fmt.)
func humanText(s string) string { return cliutil.ScrubTerminal(s) }
