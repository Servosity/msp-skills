// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Novel command scaffold. Implement the RunE body before shipping.
// generate --force preserves implemented bodies; untouched TODO scaffolds may refresh.

package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
)

func newNovelKbBrowseCmd(flags *rootFlags) *cobra.Command {

	cmd := &cobra.Command{
		Use:         "browse",
		Short:       "Print the Knowledge Base as a category/answer tree parsed from the init bundle.",
		Example:     "  zammad-cli kb browse --json",
		Annotations: map[string]string{"mcp:read-only": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				fmt.Fprintln(cmd.ErrOrStderr(), "would fetch KB")
				return nil
			}
			bundle, err := fetchZammadKBBundle(cmd, flags)
			if err != nil {
				return err
			}
			if wantsHumanTable(cmd.OutOrStdout(), flags) {
				printZammadKBTree(cmd, bundle)
				return nil
			}
			return printJSONFiltered(cmd.OutOrStdout(), struct {
				Categories []kbCategory `json:"categories"`
			}{Categories: bundle.Categories}, flags)
		},
	}
	return cmd
}

type kbBundle struct {
	Categories []kbCategory
	Answers    map[string]kbAnswer
}

type kbCategory struct {
	ID       string        `json:"id"`
	Title    string        `json:"title"`
	ParentID string        `json:"parent_id,omitempty"`
	Answers  []kbAnswerRef `json:"answers"`
}

type kbAnswerRef struct {
	ID    string `json:"id"`
	Title string `json:"title"`
}

type kbAnswer struct {
	ID         string `json:"id"`
	Title      string `json:"title"`
	CategoryID string `json:"category_id"`
	Category   string `json:"category"`
	Body       string `json:"body"`
	Published  *bool  `json:"published,omitempty"`
}

func fetchZammadKBBundle(cmd *cobra.Command, flags *rootFlags) (kbBundle, error) {
	c, err := flags.newClient()
	if err != nil {
		return kbBundle{}, err
	}
	data, status, err := c.PostQueryWithParams(cmd.Context(), "/knowledge_bases/init", nil, map[string]any{})
	if err != nil {
		return kbBundle{}, classifyAPIError(err, flags)
	}
	if status < 200 || status >= 300 {
		return kbBundle{}, fmt.Errorf("POST /knowledge_bases/init returned HTTP %d", status)
	}
	bundle, err := parseZammadKBBundle(data)
	if err != nil {
		return kbBundle{}, err
	}
	return bundle, nil
}

func parseZammadKBBundle(data json.RawMessage) (kbBundle, error) {
	var root map[string]any
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()
	if err := dec.Decode(&root); err != nil {
		return kbBundle{}, fmt.Errorf("parsing KB init response: %w", err)
	}
	preferredLocaleID := kbPreferredLocaleID(root)

	categories := make(map[string]*kbCategory)
	for key, obj := range kbSectionObjects(root, "KnowledgeBaseCategory") {
		id := kbID(obj["id"])
		if id == "" {
			id = key
		}
		if id == "" {
			continue
		}
		categories[id] = &kbCategory{
			ID:       id,
			Title:    "#" + id,
			ParentID: kbID(obj["parent_id"]),
			Answers:  make([]kbAnswerRef, 0),
		}
	}
	for categoryID, translations := range kbTranslationsByEntity(root, "KnowledgeBaseCategoryTranslation", "category_id") {
		obj := chooseKBTranslation(translations, preferredLocaleID)
		cat := categories[categoryID]
		if cat == nil {
			cat = &kbCategory{ID: categoryID, Title: "#" + categoryID, Answers: make([]kbAnswerRef, 0)}
			categories[categoryID] = cat
		}
		if title := strings.TrimSpace(stringFromAny(obj["title"])); title != "" && strings.HasPrefix(cat.Title, "#") {
			cat.Title = title
		}
	}

	answers := make(map[string]kbAnswer)
	for key, obj := range kbSectionObjects(root, "KnowledgeBaseAnswer") {
		id := kbID(obj["id"])
		if id == "" {
			id = key
		}
		if id == "" {
			continue
		}
		answer := kbAnswer{
			ID:         id,
			Title:      "#" + id,
			CategoryID: kbID(obj["category_id"]),
			Published:  kbBoolPtr(obj["published"]),
		}
		answers[id] = answer
	}
	for answerID, translations := range kbTranslationsByEntity(root, "KnowledgeBaseAnswerTranslation", "answer_id") {
		obj := chooseKBTranslation(translations, preferredLocaleID)
		answer := answers[answerID]
		if answer.ID == "" {
			answer = kbAnswer{ID: answerID, Title: "#" + answerID}
		}
		if title := strings.TrimSpace(stringFromAny(obj["title"])); title != "" {
			answer.Title = title
		}
		if body := kbTranslationBody(obj); body != "" {
			answer.Body = body
		}
		answers[answerID] = answer
	}
	for id, answer := range answers {
		if cat := categories[answer.CategoryID]; cat != nil {
			answer.Category = cat.Title
			cat.Answers = append(cat.Answers, kbAnswerRef{ID: answer.ID, Title: answer.Title})
		}
		answers[id] = answer
	}

	out := make([]kbCategory, 0, len(categories))
	for _, cat := range categories {
		sort.SliceStable(cat.Answers, func(i, j int) bool {
			if cat.Answers[i].Title != cat.Answers[j].Title {
				return cat.Answers[i].Title < cat.Answers[j].Title
			}
			return cat.Answers[i].ID < cat.Answers[j].ID
		})
		out = append(out, *cat)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].ParentID != out[j].ParentID {
			return out[i].ParentID < out[j].ParentID
		}
		if out[i].Title != out[j].Title {
			return out[i].Title < out[j].Title
		}
		return out[i].ID < out[j].ID
	})
	return kbBundle{Categories: out, Answers: answers}, nil
}

func kbPreferredLocaleID(root map[string]any) string {
	preferred := ""
	for _, obj := range kbSectionObjects(root, "KnowledgeBaseLocale") {
		primary, _ := obj["primary"].(bool)
		if !primary {
			continue
		}
		localeID := kbID(obj["id"])
		if localeID != "" && (preferred == "" || kbIDLess(localeID, preferred)) {
			preferred = localeID
		}
	}
	return preferred
}

func kbTranslationsByEntity(root map[string]any, section, entityField string) map[string][]map[string]any {
	out := make(map[string][]map[string]any)
	for _, obj := range kbSectionObjects(root, section) {
		entityID := kbID(obj[entityField])
		if entityID != "" {
			out[entityID] = append(out[entityID], obj)
		}
	}
	return out
}

func chooseKBTranslation(translations []map[string]any, preferredLocaleID string) map[string]any {
	if preferredLocaleID != "" {
		for _, obj := range translations {
			if kbID(obj["kb_locale_id"]) == preferredLocaleID {
				return obj
			}
		}
	}
	sort.SliceStable(translations, func(i, j int) bool {
		leftLocale := kbID(translations[i]["kb_locale_id"])
		rightLocale := kbID(translations[j]["kb_locale_id"])
		if leftLocale != rightLocale {
			return kbIDLess(leftLocale, rightLocale)
		}
		return kbIDLess(kbID(translations[i]["id"]), kbID(translations[j]["id"]))
	})
	if len(translations) == 0 {
		return map[string]any{}
	}
	return translations[0]
}

func kbIDLess(left, right string) bool {
	if left == "" {
		return false
	}
	if right == "" {
		return true
	}
	leftNumber, leftErr := strconv.ParseInt(left, 10, 64)
	rightNumber, rightErr := strconv.ParseInt(right, 10, 64)
	if leftErr == nil && rightErr == nil {
		return leftNumber < rightNumber
	}
	return left < right
}

func kbSectionObjects(root map[string]any, key string) map[string]map[string]any {
	raw, ok := root[key].(map[string]any)
	if !ok {
		return map[string]map[string]any{}
	}
	out := make(map[string]map[string]any, len(raw))
	for id, value := range raw {
		if obj, ok := value.(map[string]any); ok {
			out[id] = obj
		}
	}
	return out
}

func kbID(value any) string {
	return strings.TrimSpace(stringFromAny(value))
}

func kbBoolPtr(value any) *bool {
	v, ok := value.(bool)
	if !ok {
		return nil
	}
	return &v
}

func kbTranslationBody(obj map[string]any) string {
	if attrs, ok := obj["content_attributes"].(map[string]any); ok {
		if body := strings.TrimSpace(stringFromAny(attrs["body"])); body != "" {
			return body
		}
	}
	if content, ok := obj["content"].(map[string]any); ok {
		if body := strings.TrimSpace(stringFromAny(content["body"])); body != "" {
			return body
		}
	}
	return strings.TrimSpace(stringFromAny(obj["content"]))
}

func printZammadKBTree(cmd *cobra.Command, bundle kbBundle) {
	byParent := make(map[string][]kbCategory)
	for _, cat := range bundle.Categories {
		byParent[cat.ParentID] = append(byParent[cat.ParentID], cat)
	}
	var printChildren func(parentID string, depth int)
	printChildren = func(parentID string, depth int) {
		for _, cat := range byParent[parentID] {
			indent := strings.Repeat("  ", depth)
			fmt.Fprintf(cmd.OutOrStdout(), "%s- %s (%s)\n", indent, cat.Title, cat.ID)
			for _, answer := range cat.Answers {
				fmt.Fprintf(cmd.OutOrStdout(), "%s  - %s (%s)\n", indent, answer.Title, answer.ID)
			}
			printChildren(cat.ID, depth+1)
		}
	}
	printChildren("", 0)
	for _, cat := range byParent["0"] {
		_ = cat
	}
	if len(byParent[""]) == 0 && len(byParent["0"]) > 0 {
		printChildren("0", 0)
	}
}
