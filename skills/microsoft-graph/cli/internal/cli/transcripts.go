// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// pp:data-source live

package cli

import (
	"errors"
	"os"
	"strings"

	"github.com/spf13/cobra"
)

func newNovelTranscriptsCmd(flags *rootFlags) *cobra.Command {
	ann := map[string]string{"mcp:read-only": "true", "pp:data-source": "live", "pp:typed-exit-codes": "0,2"}
	if os.Getenv(mcpNoTranscriptsEnv) == "1" {
		// The MCP off switch: no transcripts tool is registered at all.
		ann["mcp:hidden"] = "true"
	}
	cmd := &cobra.Command{
		Use:   "transcripts",
		Short: "Teams meeting transcripts in your own tenant only (list, download WebVTT, all for one organizer)",
		Long: `Teams meeting transcripts, own tenant only.

An MSP must not read its clients' meetings, so every transcripts command
checks the credential before any request and refuses:
  - any token whose tenant (tid) is not MICROSOFT_GRAPH_HOME_TENANT, your own
    tenant id: a client tenant is never a transcripts target;
  - a user who signed in from another tenant (GDAP or guest);
  - an app-only token, unless MICROSOFT_GRAPH_TRANSCRIPTS_APP_ONLY=1 and a
    Teams application access policy (Grant-CsApplicationAccessPolicy) limits
    the app to named users;
  - a token whose tenant cannot be read.

Nothing is kept on disk: no local store, response cache or learn journal.
Needs OnlineMeetingTranscript.Read.All. Through MCP the commands are read-only,
return the text instead of writing files, and are switched off entirely with
MICROSOFT_GRAPH_MCP_NO_TRANSCRIPTS=1.`,
		Example:     "  microsoft-graph-cli transcripts list --meeting '<meeting-id>' --agent",
		Annotations: ann,
		RunE:        parentNoSubcommandRunE(flags),
	}
	// Plain AddCommand (not addNovelCommandIfAbsent): these files are the implementations, and the press
	// dogfood path walk only follows AddCommand (a "list" leaf otherwise reads as a depth mismatch).
	cmd.AddCommand(newNovelTranscriptsAllCmd(flags))
	cmd.AddCommand(newNovelTranscriptsDownloadCmd(flags))
	cmd.AddCommand(newNovelTranscriptsListCmd(flags))
	return cmd
}

var errTranscriptsLiveOnly = errors.New("transcripts are read live from Microsoft Graph and never synced or stored; use --data-source live or auto")

// transcriptsKeepNothing runs first in every transcripts command, before a
// dry run or any request. It refuses --data-source local (transcripts are never
// synced) and every output sink but stdout and --out: a --deliver file or
// webhook, run receipts and audit directories would each keep or send a copy
// (Codex r1 class transcript-persisted-outside-out). The values are read after
// a saved --profile is applied, so a profile cannot route them either.
func transcriptsKeepNothing(flags *rootFlags) error {
	if flags.dataSource == "local" {
		return usageErr(errTranscriptsLiveOnly)
	}
	if spec := strings.TrimSpace(flags.deliverSpec); (spec != "" && spec != "stdout") || flags.deliverBuf != nil {
		return usageErr(errors.New("--deliver is refused for transcripts: they go to stdout or the --out file you name, and nowhere else"))
	}
	if flags.receiptEnabled || strings.TrimSpace(flags.receiptFile) != "" || strings.TrimSpace(flags.auditDir) != "" {
		return usageErr(errors.New("--receipt, --receipt-file and --audit-dir are refused for transcripts: a receipt or audit log would keep the run on disk"))
	}
	return nil
}
