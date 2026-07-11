// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.
// Novel command scaffold. Implement the RunE body before shipping.
// generate --force preserves implemented bodies; untouched TODO scaffolds may refresh.

package cli

import (
	"fmt"
	"sort"
	"strings"

	"github.com/spf13/cobra"
)

func newNovelKbSearchCmd(flags *rootFlags) *cobra.Command {

	cmd := &cobra.Command{
		Use:         "search <query>",
		Short:       "Search KB answer titles and bodies from the live init bundle.",
		Example:     "  zammad-cli kb search \"restore\" --json",
		Annotations: map[string]string{"mcp:read-only": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 && cmd.Flags().NFlag() == 0 {
				return cmd.Help()
			}
			if dryRunOK(flags) {
				fmt.Fprintln(cmd.ErrOrStderr(), "would fetch KB")
				return nil
			}
			if len(args) == 0 || strings.TrimSpace(args[0]) == "" {
				return usageErr(fmt.Errorf("query is required\nUsage: %s <query>", cmd.CommandPath()))
			}
			bundle, err := fetchZammadKBBundle(cmd, flags)
			if err != nil {
				return err
			}
			query := strings.ToLower(strings.TrimSpace(args[0]))
			results := make([]kbSearchResult, 0)
			for _, answer := range bundle.Answers {
				cleanTitle := cleanZammadArticleText(answer.Title)
				cleanBody := cleanZammadArticleText(answer.Body)
				haystack := strings.ToLower(cleanTitle + "\n" + cleanBody)
				idx := strings.Index(haystack, query)
				if idx < 0 {
					continue
				}
				snippetText := cleanBody
				if strings.Contains(strings.ToLower(cleanTitle), query) {
					snippetText = cleanTitle
					idx = strings.Index(strings.ToLower(snippetText), query)
				} else {
					idx = strings.Index(strings.ToLower(snippetText), query)
				}
				results = append(results, kbSearchResult{
					ID:       answer.ID,
					Title:    answer.Title,
					Category: answer.Category,
					Snippet:  snippetAround(snippetText, idx, len(query), 120),
				})
			}
			sort.SliceStable(results, func(i, j int) bool {
				if results[i].Title != results[j].Title {
					return results[i].Title < results[j].Title
				}
				return results[i].ID < results[j].ID
			})
			return printJSONFiltered(cmd.OutOrStdout(), results, flags)
		},
	}
	return cmd
}

type kbSearchResult struct {
	ID       string `json:"id"`
	Title    string `json:"title"`
	Category string `json:"category"`
	Snippet  string `json:"snippet"`
}
