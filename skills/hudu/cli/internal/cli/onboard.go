// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-authored transcendence feature: company onboarding scaffold. Applies a
// saved bundle of asset layouts, folders, and procedures-from-template to a new
// company in one idempotent op, so every client gets the identical house-standard
// documentation skeleton. Prints the plan by default; --apply executes the writes.

// pp:data-source live

package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
	"hudu-pp-cli/internal/cliutil"
)

type onboardTemplate struct {
	AssetLayouts       []map[string]any `json:"asset_layouts"`
	Folders            []map[string]any `json:"folders"`
	ProcedureTemplates []int            `json:"procedure_templates"`
}

type onboardStep struct {
	Action string `json:"action"`
	Detail string `json:"detail"`
	Path   string `json:"path"`
}

func templatesDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return filepath.Join(".", "hudu-templates")
	}
	return filepath.Join(home, ".config", "hudu-cli", "templates")
}

func resolveTemplatePath(nameOrPath string) string {
	if nameOrPath == "" {
		return ""
	}
	if _, err := os.Stat(nameOrPath); err == nil {
		return nameOrPath
	}
	cand := filepath.Join(templatesDir(), nameOrPath+".json")
	return cand
}

func newNovelOnboardCmd(flags *rootFlags) *cobra.Command {
	var flagCompany int
	var flagTemplate string
	var flagApply bool
	var flagInit bool

	cmd := &cobra.Command{
		Use:         "onboard",
		Short:       "Apply a saved bundle of asset layouts, folder tree, and procedures-from-template to a new company in one command.",
		Annotations: map[string]string{"mcp:read-only": "false"},
		Long: `Scaffold a new client's documentation from a saved template so every onboarding
produces the identical house-standard structure (no per-tech drift).

A template is a JSON file with optional "asset_layouts", "folders", and
"procedure_templates" (template procedure ids to instantiate). Resolve it by
path or by name under ` + "`~/.config/hudu-cli/templates/<name>.json`" + `.

By default the command prints the ordered plan of what it WOULD create (no API
calls). Pass --apply to execute the writes against the live API. Pass --init to
write an example template you can edit.`,
		Example: `  # Write an example template to edit
  hudu-cli onboard --init --template msp-standard

  # Preview the plan for company 42 (no writes)
  hudu-cli onboard --company 42 --template msp-standard

  # Execute the scaffold
  hudu-cli onboard --company 42 --template msp-standard --apply`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if flagInit {
				return onboardInit(cmd, flags, flagTemplate)
			}
			if dryRunOK(flags) {
				return nil
			}
			if flagCompany <= 0 {
				return usageErr(fmt.Errorf("--company <id> is required"))
			}
			if flagTemplate == "" {
				return usageErr(fmt.Errorf("--template <name-or-path> is required (use --init to create one)"))
			}
			tplPath := resolveTemplatePath(flagTemplate)
			// #nosec G304 -- tplPath is the user-supplied --template flag (a name-or-path the operator controls); reading it is the command's purpose.
			raw, err := os.ReadFile(tplPath)
			if err != nil {
				return usageErr(fmt.Errorf("reading template %q: %w\nUse 'hudu-cli onboard --init --template %s' to create one.", tplPath, err, flagTemplate))
			}
			var tpl onboardTemplate
			if err := json.Unmarshal(raw, &tpl); err != nil {
				return usageErr(fmt.Errorf("parsing template %q: %w", tplPath, err))
			}

			// Build the ordered plan.
			var plan []onboardStep
			for _, l := range tpl.AssetLayouts {
				plan = append(plan, onboardStep{Action: "create-asset-layout", Detail: asString(l["name"]), Path: "/asset_layouts"})
			}
			for _, f := range tpl.Folders {
				plan = append(plan, onboardStep{Action: "create-folder", Detail: asString(f["name"]), Path: "/folders"})
			}
			for _, pid := range tpl.ProcedureTemplates {
				plan = append(plan, onboardStep{Action: "create-procedure-from-template", Detail: fmt.Sprintf("template #%d", pid), Path: fmt.Sprintf("/procedures/%d/create_from_template", pid)})
			}

			// Execute only when --apply AND not in verify/dry-run.
			execute := flagApply && !cliutil.IsVerifyEnv() && !flags.dryRun
			if !execute {
				return emitAudit(cmd, flags, map[string]any{
					"company_id": flagCompany,
					"template":   tplPath,
					"apply":      false,
					"plan":       plan,
				}, func(w io.Writer) {
					if len(plan) == 0 {
						fmt.Fprintf(w, "Template %q has no steps.\n", tplPath)
						return
					}
					fmt.Fprintf(w, "Onboarding plan for company %d (template %s) — %d step(s), not yet applied:\n", flagCompany, tplPath, len(plan))
					for i, s := range plan {
						fmt.Fprintf(w, "  %d. would %s: %s\n", i+1, s.Action, s.Detail)
					}
					fmt.Fprintln(w, "Re-run with --apply to execute.")
				})
			}

			// Live execution.
			if err := validateDataSourceStrategy(flags, "live"); err != nil {
				return err
			}
			c, err := flags.newClient()
			if err != nil {
				return err
			}
			var results []map[string]any
			for _, l := range tpl.AssetLayouts {
				body := cloneMap(l)
				data, status, err := c.PostWithParams(cmd.Context(), "/asset_layouts", map[string]string{}, body)
				results = append(results, applyResult("create-asset-layout", asString(l["name"]), status, err, data))
				if err != nil {
					return finishApply(cmd, flags, results, classifyAPIError(err, flags))
				}
			}
			for _, f := range tpl.Folders {
				body := cloneMap(f)
				if _, ok := body["company_id"]; !ok {
					body["company_id"] = flagCompany
				}
				data, status, err := c.PostWithParams(cmd.Context(), "/folders", map[string]string{}, body)
				results = append(results, applyResult("create-folder", asString(f["name"]), status, err, data))
				if err != nil {
					return finishApply(cmd, flags, results, classifyAPIError(err, flags))
				}
			}
			for _, pid := range tpl.ProcedureTemplates {
				body := map[string]any{"company_id": flagCompany}
				path := fmt.Sprintf("/procedures/%d/create_from_template", pid)
				data, status, err := c.PostWithParams(cmd.Context(), path, map[string]string{}, body)
				results = append(results, applyResult("create-procedure-from-template", fmt.Sprintf("template #%d", pid), status, err, data))
				if err != nil {
					return finishApply(cmd, flags, results, classifyAPIError(err, flags))
				}
			}
			return finishApply(cmd, flags, results, nil)
		},
	}
	cmd.Flags().IntVar(&flagCompany, "company", 0, "Target company id")
	cmd.Flags().StringVar(&flagTemplate, "template", "", "Template name (under ~/.config/hudu-cli/templates) or path to a JSON file")
	cmd.Flags().BoolVar(&flagApply, "apply", false, "Execute the scaffold against the live API (default: print the plan only)")
	cmd.Flags().BoolVar(&flagInit, "init", false, "Write an example template and exit")
	return cmd
}

func cloneMap(m map[string]any) map[string]any {
	out := make(map[string]any, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out
}

func applyResult(action, detail string, status int, err error, data json.RawMessage) map[string]any {
	r := map[string]any{"action": action, "detail": detail, "status": status, "success": err == nil && status >= 200 && status < 300}
	if err != nil {
		r["error"] = err.Error()
	}
	if id := createdObjectID(data); id > 0 {
		r["id"] = id
	}
	return r
}

// createdObjectID pulls the created object's id from a POST response body,
// unwrapping Hudu's single-key envelopes ({"asset_layout": {...}}).
func createdObjectID(data json.RawMessage) int {
	var m map[string]any
	if json.Unmarshal(data, &m) != nil {
		return 0
	}
	if id := intField(m, "id"); id > 0 {
		return id
	}
	if len(m) == 1 {
		for _, v := range m {
			if inner, ok := v.(map[string]any); ok {
				return intField(inner, "id")
			}
		}
	}
	return 0
}

func finishApply(cmd *cobra.Command, flags *rootFlags, results []map[string]any, runErr error) error {
	_ = emitAudit(cmd, flags, map[string]any{"apply": true, "results": results}, func(w io.Writer) {
		for _, r := range results {
			status := "ok"
			if r["success"] != true {
				status = "FAILED"
			}
			fmt.Fprintf(w, "  [%s] %s: %s\n", status, r["action"], r["detail"])
		}
	})
	return runErr
}

func onboardInit(cmd *cobra.Command, flags *rootFlags, name string) error {
	if name == "" {
		name = "msp-standard"
	}
	example := onboardTemplate{
		AssetLayouts: []map[string]any{
			{"name": "Server", "icon": "fas fa-server", "fields": []map[string]any{
				{"label": "Hostname", "field_type": "Text", "required": true, "position": 1},
				{"label": "IP Address", "field_type": "Text", "required": true, "position": 2},
				{"label": "Role", "field_type": "Text", "required": false, "position": 3},
			}},
		},
		Folders:            []map[string]any{{"name": "Standard Operating Procedures", "icon": "fas fa-book"}},
		ProcedureTemplates: []int{},
	}
	dir := templatesDir()
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return fmt.Errorf("creating templates dir: %w", err)
	}
	path := filepath.Join(dir, name+".json")
	buf, _ := json.MarshalIndent(example, "", "  ")
	if err := os.WriteFile(path, buf, 0o600); err != nil {
		return fmt.Errorf("writing template: %w", err)
	}
	return emitAudit(cmd, flags, map[string]any{"wrote": path}, func(w io.Writer) {
		fmt.Fprintf(w, "Wrote example template to %s\nEdit it, then run: hudu-cli onboard --company <id> --template %s\n", path, name)
	})
}
