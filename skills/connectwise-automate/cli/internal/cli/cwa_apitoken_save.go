// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

// Hand-written (handfixes.json apitoken-minted-token-saved).

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"connectwise-automate-pp-cli/internal/config"
)

// saveMintedToken handles the TokenResult of POST /apitoken (apitoken mint)
// and POST /apitoken/refresh (apitoken refresh): its AccessToken is the bearer
// token. Printed, it landed in the terminal, the agent's context and the local
// SQLite mirror; the credential kit holds it (Declarations.ResponseFields), so
// the whole answer would be withheld instead. So it is saved to the 0600
// credential store, as `auth set-token` saves a token, and the answer goes on
// to the mirror and the output with AccessToken naming where it went. Every
// later command authenticates with it unless CONNECTWISE_AUTOMATE_TOKEN is set.
// An answer without an AccessToken (a dry run, an error) passes unchanged; a
// token that cannot be saved fails the command.
func saveMintedToken(cmd *cobra.Command, flags *rootFlags, data json.RawMessage, status int) (json.RawMessage, error) {
	if flags.dryRun || status < 200 || status >= 300 {
		return data, nil
	}
	var answer map[string]any
	if json.Unmarshal(data, &answer) != nil {
		return data, nil
	}
	key, token := "", ""
	for k, v := range answer {
		if s, ok := v.(string); ok && strings.EqualFold(k, "AccessToken") && strings.TrimSpace(s) != "" {
			key, token = k, strings.TrimSpace(s)
		}
	}
	if key == "" {
		return data, nil
	}
	cfg, err := config.Load(flags.configPath)
	if err != nil {
		return nil, configErr(err)
	}
	// As auth set-token: a legacy auth_header would shadow the saved token.
	cfg.AuthHeaderVal = ""
	if err := cfg.SaveTokens("", "", token, "", time.Time{}); err != nil {
		return nil, configErr(fmt.Errorf("saving the minted token: %w", err))
	}
	answer[key] = "[saved to " + credentialSavePath(cfg) + "]"
	out, err := json.Marshal(answer)
	if err != nil {
		return nil, err
	}
	if cmd != nil && !flags.asJSON && !flags.quiet {
		fmt.Fprintf(cmd.ErrOrStderr(), "Token saved to %s; later commands use it unless CONNECTWISE_AUTOMATE_TOKEN is set.\n", credentialSavePath(cfg))
	}
	return out, nil
}
